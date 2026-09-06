package query

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"slices"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

// Valve's A2S query protocol, shared by GoldSource (Half-Life 1 and its mods) and Source servers.
//
// A GoldSource server running ReUnion answers A2S_INFO according to its ServerInfoAnswerType setting:
// 0 sends the Source-style 'I' reply, 1 sends the old GoldSource-style 'm' reply, and 2 ("hybrid") sends
// both back to back, sometimes with an empty players reply in between. Both info formats are therefore
// accepted, and the info and players requests go over separate sockets: once the info socket is closed the
// extra hybrid packets are dropped by the kernel instead of being mistaken for the players reply.
const (
	a2sHeaderChallenge      byte = 'A'
	a2sHeaderInfoSource     byte = 'I'
	a2sHeaderInfoGoldSource byte = 'm'
	a2sHeaderPlayers        byte = 'D'
	a2sHeaderRules          byte = 'E'

	a2sSimplePacketHeader uint32 = 0xFFFFFFFF
	a2sSplitPacketHeader  uint32 = 0xFFFFFFFE
	a2sSplitCompressedBit uint32 = 0x80000000

	// a2sMinPacketSize is the packet header followed by the reply type byte.
	a2sMinPacketSize = 5
	// a2sSplitHeaderSize covers the split header: packet header, id, total, number and fragment size.
	a2sSplitHeaderSize = 12
	a2sChallengeSize   = 4
)

var (
	a2sInfoRequestPrefix    = []byte("\xFF\xFF\xFF\xFFTSource Engine Query\x00")
	a2sPlayerRequestPrefix  = []byte{0xFF, 0xFF, 0xFF, 0xFF, 'U'}
	a2sChallengePlaceholder = []byte{0xFF, 0xFF, 0xFF, 0xFF}
)

type a2sServerInfo struct {
	name       string
	gameMap    string
	players    uint8
	maxPlayers uint8
}

func querySource(ctx context.Context, host string, port int) (*Result, error) {
	result := &Result{
		Online:    false,
		QueryTime: time.Now(),
	}

	address := net.JoinHostPort(host, strconv.Itoa(port))
	deadline := a2sDeadline(ctx)

	info, err := queryA2SInfo(ctx, address, deadline)
	if err != nil {
		return result, errors.WithMessage(err, "failed to query info")
	}

	result.Online = true
	result.Name = info.name
	result.Map = info.gameMap
	result.PlayersNum = int(info.players)
	result.MaxPlayersNum = int(info.maxPlayers)

	players, err := queryA2SPlayers(ctx, address, deadline)
	if err != nil {
		return result, errors.WithMessage(err, "failed to query players")
	}

	result.Players = players

	return result, nil
}

func queryA2SInfo(ctx context.Context, address string, deadline time.Time) (a2sServerInfo, error) {
	conn, release, err := dialA2S(ctx, address, deadline)
	if err != nil {
		return a2sServerInfo{}, err
	}
	defer release()

	packet, err := exchangeA2S(conn, buildA2SInfoRequest, a2sHeaderInfoSource, a2sHeaderInfoGoldSource)
	if err != nil {
		return a2sServerInfo{}, err
	}

	info, err := parseA2SInfo(packet)
	if err != nil {
		return a2sServerInfo{}, errors.WithMessage(err, "failed to parse response")
	}

	return info, nil
}

func queryA2SPlayers(ctx context.Context, address string, deadline time.Time) ([]ResultPlayer, error) {
	conn, release, err := dialA2S(ctx, address, deadline)
	if err != nil {
		return nil, err
	}
	defer release()

	packet, err := exchangeA2S(conn, buildA2SPlayerRequest, a2sHeaderPlayers)
	if err != nil {
		return nil, err
	}

	players, err := parseA2SPlayers(packet)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse response")
	}

	return players, nil
}

// a2sDeadline bounds the whole query, both sockets included: the context deadline when there is one, the
// package default otherwise.
func a2sDeadline(ctx context.Context) time.Time {
	if deadline, ok := ctx.Deadline(); ok {
		return deadline
	}

	return time.Now().Add(defaultTimeout)
}

// dialA2S opens a UDP socket to the server with every read and write bounded by deadline. The returned release
// func closes the socket. Cancelling ctx before that interrupts a blocked read at once instead of leaving it to
// run into the deadline.
func dialA2S(ctx context.Context, address string, deadline time.Time) (net.Conn, func(), error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp", address)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create UDP connection")
	}

	err = conn.SetDeadline(deadline)
	if err != nil {
		_ = conn.Close()

		return nil, nil, errors.Wrap(err, "failed to set deadline")
	}

	stop := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
	})

	release := func() {
		stop()
		_ = conn.Close()
	}

	return conn, release, nil
}

// buildA2SInfoRequest builds A2S_INFO; the challenge, once the server hands one out, is appended.
func buildA2SInfoRequest(challenge []byte) []byte {
	return slices.Concat(a2sInfoRequestPrefix, challenge)
}

// buildA2SPlayerRequest builds A2S_PLAYER; without a challenge the request carries the -1 placeholder.
func buildA2SPlayerRequest(challenge []byte) []byte {
	if challenge == nil {
		challenge = a2sChallengePlaceholder
	}

	return slices.Concat(a2sPlayerRequestPrefix, challenge)
}

// exchangeA2S sends a request and reads until a reply of an accepted type arrives. A challenge reply makes
// the request go out once more with the challenge included. Replies of another known type are skipped
// rather than failing the query: a ReUnion server in hybrid mode answers one info request with several packets,
// and they may arrive in any order.
func exchangeA2S(conn net.Conn, build func(challenge []byte) []byte, accept ...byte) ([]byte, error) {
	_, err := conn.Write(build(nil))
	if err != nil {
		return nil, errors.Wrap(err, "failed to send query packet")
	}

	challenged := false

	for {
		packet, err := readA2SPacket(conn)
		if err != nil {
			return nil, err
		}

		kind := packet[4]

		switch {
		case slices.Contains(accept, kind):
			return packet, nil
		case kind == a2sHeaderChallenge:
			if challenged {
				return nil, errors.New("server rejected the challenge it handed out")
			}

			if len(packet) < a2sMinPacketSize+a2sChallengeSize {
				return nil, errors.New("challenge response too short")
			}

			challenged = true

			_, err = conn.Write(build(packet[a2sMinPacketSize : a2sMinPacketSize+a2sChallengeSize]))
			if err != nil {
				return nil, errors.Wrap(err, "failed to send query packet")
			}
		case kind == a2sHeaderInfoSource, kind == a2sHeaderInfoGoldSource,
			kind == a2sHeaderPlayers, kind == a2sHeaderRules:
			continue
		default:
			return nil, errors.Errorf("unexpected response type 0x%02x", kind)
		}
	}
}

// readA2SPacket reads one logical reply: a single datagram, or a Source-style split reply reassembled from all
// of its fragments. The returned packet starts with the -1 header followed by the reply type byte.
func readA2SPacket(conn net.Conn) ([]byte, error) {
	packet, err := readA2SDatagram(conn)
	if err != nil {
		return nil, err
	}

	if binary.LittleEndian.Uint32(packet) == a2sSplitPacketHeader {
		packet, err = reassembleA2SSplitPacket(conn, packet)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to reassemble split response")
		}
	}

	if len(packet) < a2sMinPacketSize || binary.LittleEndian.Uint32(packet) != a2sSimplePacketHeader {
		return nil, errors.New("invalid response header")
	}

	return packet, nil
}

func readA2SDatagram(conn net.Conn) ([]byte, error) {
	buffer := make([]byte, defaultMaxPacketSize)

	n, err := conn.Read(buffer)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read query response")
	}

	if n < a2sMinPacketSize {
		return nil, errors.New("response too short")
	}

	return buffer[:n], nil
}

type a2sSplitFragment struct {
	id      uint32
	total   uint8
	number  uint8
	payload []byte
}

// parseA2SSplitFragment reads the Source split header: -2, id, total, number, fragment size. The two-byte size
// field is skipped; the payload is whatever follows it.
//
// The GoldSource layout (id, then a single byte holding the number and the total in its two nibbles) is
// deliberately not handled: GoldSource servers split only the rules reply. An info reply is small, and a players
// reply is at most 32 players of 1 + 32 + 4 + 4 bytes plus a 6-byte header, 1318 bytes, under the 1400-byte split
// threshold. Were such a fragment still read here, fragment 0 puts the 0xFF of its payload header into the number
// field and the set fails the range and consistency checks in reassembleA2SSplitPacket instead of yielding data.
func parseA2SSplitFragment(datagram []byte) (a2sSplitFragment, error) {
	if len(datagram) < a2sSplitHeaderSize {
		return a2sSplitFragment{}, errors.New("split fragment too short")
	}

	if binary.LittleEndian.Uint32(datagram) != a2sSplitPacketHeader {
		return a2sSplitFragment{}, errors.New("invalid split fragment header")
	}

	return a2sSplitFragment{
		id:      binary.LittleEndian.Uint32(datagram[4:]),
		total:   datagram[8],
		number:  datagram[9],
		payload: datagram[a2sSplitHeaderSize:],
	}, nil
}

// reassembleA2SSplitPacket collects the remaining fragments of a split reply whose first fragment has already
// been read. bzip2-compressed replies are refused: no current server sends them.
func reassembleA2SSplitPacket(conn net.Conn, first []byte) ([]byte, error) {
	fragment, err := parseA2SSplitFragment(first)
	if err != nil {
		return nil, err
	}

	if fragment.id&a2sSplitCompressedBit != 0 {
		return nil, errors.New("compressed split response is not supported")
	}

	if fragment.total == 0 {
		return nil, errors.New("split response declares no fragments")
	}

	id, total := fragment.id, fragment.total
	fragments := make([][]byte, total)
	received := 0

	for {
		if fragment.number >= total {
			return nil, errors.Errorf("split fragment %d is out of range for %d fragments", fragment.number, total)
		}

		if fragments[fragment.number] != nil {
			return nil, errors.Errorf("duplicate split fragment %d", fragment.number)
		}

		fragments[fragment.number] = fragment.payload
		received++

		if received == int(total) {
			return bytes.Join(fragments, nil), nil
		}

		datagram, err := readA2SDatagram(conn)
		if err != nil {
			return nil, err
		}

		fragment, err = parseA2SSplitFragment(datagram)
		if err != nil {
			return nil, err
		}

		if fragment.id != id || fragment.total != total {
			return nil, errors.New("split fragment belongs to another response")
		}
	}
}

func parseA2SInfo(packet []byte) (a2sServerInfo, error) {
	fields := a2sFieldReader{reader: bytes.NewReader(packet[a2sMinPacketSize:])}

	switch packet[4] {
	case a2sHeaderInfoSource:
		return parseA2SSourceInfo(&fields)
	case a2sHeaderInfoGoldSource:
		return parseA2SGoldSourceInfo(&fields)
	default:
		return a2sServerInfo{}, errors.Errorf("unexpected info response type 0x%02x", packet[4])
	}
}

// parseA2SSourceInfo reads the 'I' reply up to the player counts. The fields after them (bots, server type,
// OS, password, VAC, version, extra data) are not needed and are left unread.
func parseA2SSourceInfo(fields *a2sFieldReader) (a2sServerInfo, error) {
	fields.skip(1) // protocol version

	info := a2sServerInfo{
		name:    fields.readString(),
		gameMap: fields.readString(),
	}

	fields.readString() // game folder
	fields.readString() // game description
	fields.skip(2)      // application id

	info.players = fields.readByte()
	info.maxPlayers = fields.readByte()

	return info, fields.result()
}

// parseA2SGoldSourceInfo reads the 'm' reply up to the player counts. The fields after them (protocol,
// server type, OS, password, the optional mod block, VAC, bots) are not needed and are left unread.
func parseA2SGoldSourceInfo(fields *a2sFieldReader) (a2sServerInfo, error) {
	fields.readString() // server address

	info := a2sServerInfo{
		name:    fields.readString(),
		gameMap: fields.readString(),
	}

	fields.readString() // game folder
	fields.readString() // game description

	info.players = fields.readByte()
	info.maxPlayers = fields.readByte()

	return info, fields.result()
}

func parseA2SPlayers(packet []byte) ([]ResultPlayer, error) {
	fields := a2sFieldReader{reader: bytes.NewReader(packet[a2sMinPacketSize:])}

	count := fields.readByte()
	players := make([]ResultPlayer, 0, count)

	for range count {
		fields.skip(1) // index of the player chunk

		name := fields.readString()
		score := fields.readInt32()

		fields.skip(4) // time connected, float32

		if fields.err != nil {
			break
		}

		players = append(players, ResultPlayer{
			Name:  name,
			Score: int(score),
		})
	}

	err := fields.result()
	if err != nil {
		return nil, err
	}

	return players, nil
}

// a2sFieldReader reads wire fields in sequence and keeps the first failure, so a parser can be written as a
// straight list of fields and check for a truncated packet once at the end.
type a2sFieldReader struct {
	reader *bytes.Reader
	err    error
}

func (r *a2sFieldReader) readString() string {
	if r.err != nil {
		return ""
	}

	value, err := readNullTerminatedString(r.reader)
	r.err = err

	return value
}

func (r *a2sFieldReader) readByte() byte {
	if r.err != nil {
		return 0
	}

	value, err := r.reader.ReadByte()
	r.err = err

	return value
}

func (r *a2sFieldReader) readInt32() int32 {
	if r.err != nil {
		return 0
	}

	var value int32
	r.err = binary.Read(r.reader, binary.LittleEndian, &value)

	return value
}

func (r *a2sFieldReader) skip(n int) {
	if r.err != nil {
		return
	}

	_, r.err = io.CopyN(io.Discard, r.reader, int64(n))
}

func (r *a2sFieldReader) result() error {
	if r.err != nil {
		return errors.Wrap(r.err, "truncated response")
	}

	return nil
}
