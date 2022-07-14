package types

type Stats struct {
	Samples int   `json:"samples"`
	Min     Float `json:"min"`
	Avg     Float `json:"avg"`
	Max     Float `json:"max"`
}
