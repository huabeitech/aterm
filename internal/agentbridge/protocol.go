package agentbridge

type Request struct {
	Method string
	HostID string
	Query  string
}

type Host struct {
	ID      string `json:"hostId"`
	Name    string `json:"name"`
	Alias   string `json:"alias"`
	Group   string `json:"group,omitempty"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	User    string `json:"user"`
	Remark  string `json:"remark"`
}

type Connection struct {
	Program              string            `json:"program"`
	Args                 []string          `json:"args"`
	Env                  map[string]string `json:"env"`
	AuthMode             string            `json:"authMode"`
	RequiresUserTerminal bool              `json:"requiresUserTerminal"`
}
type Result struct {
	Hosts      []Host      `json:"hosts,omitempty"`
	Connection *Connection `json:"connection,omitempty"`
	Error      string      `json:"error,omitempty"`
}
