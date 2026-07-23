package extensions

// Entry is one extension → email (UPN) mapping used for Graph presence.
type Entry struct {
	Extension string `json:"extension"`
	Email     string `json:"email"`
}
