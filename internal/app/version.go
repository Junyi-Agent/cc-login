package app

// Setup is how claude starts on this machine, for cc-login version.
type Setup struct {
	Claude    string // the claude cc-login runs
	ClaudeErr error  // why there is none
	InPath    string // what a plain claude runs, the first claude in PATH; "" for none
	Linked    bool   // InPath is cc-login: the claude link (ADR 0007)
}

// Setup says which claude cc-login runs, and whether a plain claude goes
// through cc-login.
func (a *App) Setup() Setup {
	var s Setup
	s.Claude, s.ClaudeErr = a.Claude.Path()
	s.InPath, s.Linked = a.Claude.InPath()
	return s
}
