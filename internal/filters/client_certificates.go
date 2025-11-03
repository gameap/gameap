package filters

type FindClientCertificate struct {
	IDs []uint
}

func FindClientCertificateByIDs(ids ...uint) *FindClientCertificate {
	return &FindClientCertificate{
		IDs: ids,
	}
}
