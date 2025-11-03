package getbusyports

type busyPortsResponse map[string][]int

func newBusyPortsResponse(busyPorts map[string][]int) busyPortsResponse {
	if busyPorts == nil {
		return busyPortsResponse{}
	}

	return busyPortsResponse(busyPorts)
}
