package query

type UnsupportedQueryProtocolError Protocol

func NewUnsupportedQueryProtocolError(protocol Protocol) UnsupportedQueryProtocolError {
	return UnsupportedQueryProtocolError(protocol)
}

func (er UnsupportedQueryProtocolError) Error() string {
	return "unsupported query protocol: " + string(er)
}
