package domain

// Panel-side settings that live in the generic Node.Metadata bag rather than
// in dedicated columns. Access only through the accessors below so the key
// names never leak into handlers.
const (
	metaKeyPortRange = "port_range"
)

// PortRange returns the pool the panel picks ports from for servers created on
// this node without them; an empty pool when none is configured.
func (n *Node) PortRange() (PortRange, error) {
	return portRangeFromMetadata(n.Metadata)
}

// portRangeFromMetadata treats an absent or null entry as no pool; anything
// else must be a string ParsePortRange accepts.
func portRangeFromMetadata(metadata Metadata) (PortRange, error) {
	raw, ok := metadata[metaKeyPortRange]
	if !ok || raw == nil {
		return nil, nil
	}

	value, ok := raw.(string)
	if !ok {
		return nil, ErrPortRangeInvalid
	}

	return ParsePortRange(value)
}
