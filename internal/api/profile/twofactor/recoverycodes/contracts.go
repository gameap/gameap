package recoverycodes

// twoFactorManager is the slice of the two-factor manager this handler needs:
// mint a fresh recovery-code set (the old set is discarded by overwrite).
type twoFactorManager interface {
	GenerateRecoveryCodes() (plain []string, encoded string, err error)
}
