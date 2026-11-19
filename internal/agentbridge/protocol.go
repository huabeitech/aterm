package agentbridge

type Request struct {
	Method string
	HostID string
	Query  string
}
type Host struct {
	ID    string `json:"hostId"`
	Name  string `json:"name"`
	Group string `json:"group,omitempty"`
}
type Result struct {
	Hosts  []Host `json:"hosts,omitempty"`
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}
