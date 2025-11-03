package domain

import "time"

type ClientCertificate struct {
	ID          uint      `db:"id"`
	Fingerprint string    `db:"fingerprint"`
	Expires     time.Time `db:"expires"`
	Certificate string    `db:"certificate"`
	PrivateKey  string    `db:"private_key"`
}
