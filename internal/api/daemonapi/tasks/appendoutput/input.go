package appendoutput

type appendOutputInput struct {
	Output string `json:"output"`
}

func (in *appendOutputInput) Validate() error {
	return nil
}
