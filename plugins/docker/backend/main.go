package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	sdk "ssh-connect/packages/plugin-go"
)

const logBytes = 24576
const logLines = 300
const infoProjection = `{"ID":{{json .ID}},"Name":{{json .Name}},"ServerVersion":{{json .ServerVersion}}}`
const containerProjection = `{"Id":{{json .Id}},"Name":{{json .Name}},"Created":{{json .Created}},"State":{"Status":{{json .State.Status}},"Health":{{with index .State "Health"}}{"Status":{{json .Status}}}{{else}}null{{end}}},"Config":{"Image":{{json .Config.Image}}},"HostConfig":{"RestartPolicy":{{json .HostConfig.RestartPolicy}}},"Mounts":[{{range $i,$m := .Mounts}}{{if $i}},{{end}}{"Source":{{json $m.Source}},"Destination":{{json $m.Destination}},"RW":{{json $m.RW}}}{{end}}],"NetworkSettings":{"Ports":{{json .NetworkSettings.Ports}},"Networks":{ {{$first := true}}{{range $name,$network := .NetworkSettings.Networks}}{{if not $first}},{{end}}{{$first = false}}{{json $name}}:{}{{end}} }}}`
const imageProjection = `{"Id":{{json .Id}},"RepoTags":{{json .RepoTags}},"RepoDigests":{{json .RepoDigests}},"Created":{{json .Created}},"Size":{{json .Size}},"Os":{{json .Os}},"Architecture":{{json .Architecture}},"LayersCount":{{len .RootFS.Layers}},"Config":{"Entrypoint":{{json .Config.Entrypoint}},"Cmd":{{json .Config.Cmd}}}}`

var contextName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,199}$`)
var containerID = regexp.MustCompile(`^[a-f0-9]{12,64}$`)
var imageID = regexp.MustCompile(`^(sha256:)?[a-f0-9]{12,64}$`)
var immutableID = regexp.MustCompile(`^[a-f0-9]{64}$`)
var apiMismatch = regexp.MustCompile(`(?i)client version \d+\.\d+ is too (new|old)`)
var apiVersion = regexp.MustCompile(`(?i)(maximum supported API version is|minimum supported API version is)\s*(\d+\.\d+)`)

type request struct {
	Operation string `json:"operation"`
	ID        string `json:"id,omitempty"`
	Context   string `json:"context,omitempty"`
	Engine    string `json:"engine,omitempty"`
	Shell     string `json:"shell,omitempty"`
}

func decodeRequest(raw json.RawMessage) (request, error) {
	var r request
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if err := d.Decode(&r); err != nil {
		return r, errors.New("Invalid Docker request")
	}
	if !strings.Contains("|connect|list|inspect|logs|start|stop|restart|remove|exec|images.list|images.inspect|images.remove|", "|"+r.Operation+"|") {
		return r, errors.New("Invalid Docker operation")
	}
	if r.Operation != "connect" && (!contextName.MatchString(r.Context) || r.Engine == "" || len(r.Engine) > 200) {
		return r, errors.New("Reconnect to the Docker engine first")
	}
	if r.Operation != "connect" && r.Operation != "list" && r.Operation != "images.list" {
		valid := containerID.MatchString(r.ID)
		if strings.HasPrefix(r.Operation, "images.") {
			valid = imageID.MatchString(r.ID)
		}
		if !valid {
			return r, errors.New("Invalid container ID")
		}
	}
	if r.Shell != "" && r.Shell != "sh" && r.Shell != "bash" {
		return r, errors.New("Invalid container shell")
	}
	return r, nil
}
func arguments(r request) []string {
	base := []string{"--context", r.Context}
	var args []string
	switch r.Operation {
	case "list":
		args = []string{"ps", "-a", "--no-trunc", "--format", `{"ID":{{json .ID}},"Names":{{json .Names}},"Image":{{json .Image}},"State":{{json .State}},"Status":{{json .Status}},"Ports":{{json .Ports}}}`}
	case "inspect":
		args = []string{"inspect", "--type", "container", "--format", containerProjection, r.ID}
	case "logs":
		args = []string{"logs", "--timestamps", "--tail", "300", r.ID}
	case "images.list":
		args = []string{"image", "ls", "--no-trunc", "--format", "{{json .}}"}
	case "images.inspect":
		args = []string{"image", "inspect", "--format", imageProjection, r.ID}
	case "images.remove":
		args = []string{"image", "rm", r.ID}
	case "remove":
		args = []string{"rm", r.ID}
	case "start":
		args = []string{"start", r.ID}
	case "stop", "restart":
		args = []string{r.Operation, "--time", "10", r.ID}
	case "exec":
		shell := r.Shell
		if shell == "" {
			shell = "sh"
		}
		args = []string{"exec", "-it", r.ID, shell}
	}
	return append(base, args...)
}
func mutation(op string) bool {
	return op == "start" || op == "stop" || op == "restart" || op == "remove" || op == "exec" || op == "images.remove"
}

type environment struct {
	value   map[string]*string
	expires time.Time
}
type logState struct {
	subscription, id, stream, text string
	truncated, ended               bool
	failure                        string
}
type session struct {
	op                 sync.Mutex
	mu                 sync.Mutex
	env                map[string]environment
	verified           map[string]time.Time
	availability       map[string]any
	availableUntil     time.Time
	availabilityReason string
	log                *logState
	closed             bool
}
type backend struct {
	runtime  *sdk.SessionRuntime
	mu       sync.Mutex
	sessions map[string]*session
	streams  map[string]string
	sequence uint64
}

func newBackend(r *sdk.SessionRuntime) *backend {
	return &backend{runtime: r, sessions: map[string]*session{}, streams: map[string]string{}}
}
func main() {
	sdk.ServeSession(func(r *sdk.SessionRuntime) sdk.SessionProgram { return newBackend(r) })
}
func (b *backend) state(token string) *session {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.sessions[token]
	if s == nil {
		s = &session{env: map[string]environment{}, verified: map[string]time.Time{}}
		b.sessions[token] = s
	}
	return s
}
func (b *backend) Tasks() int { b.mu.Lock(); defer b.mu.Unlock(); return len(b.streams) }
func (b *backend) Close() {
	b.mu.Lock()
	tokens := []string{}
	for token := range b.sessions {
		tokens = append(tokens, token)
	}
	b.mu.Unlock()
	for _, token := range tokens {
		b.Release(token)
	}
}
func (b *backend) Release(token string) {
	b.mu.Lock()
	s := b.sessions[token]
	delete(b.sessions, token)
	for id, t := range b.streams {
		if t == token {
			delete(b.streams, id)
		}
	}
	b.mu.Unlock()
	if s != nil {
		s.mu.Lock()
		s.closed = true
		log := s.log
		s.log = nil
		s.mu.Unlock()
		if log != nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = b.runtime.Capability(ctx, "process.stream.stop", map[string]string{"token": token, "id": log.stream}, nil)
		}
	}
}
func exitError(p sdk.ProcessResult) error {
	if p.ExitCode != nil && *p.ExitCode == 0 {
		return nil
	}
	message := strings.TrimSpace(p.Stderr)
	if message == "" {
		if p.ExitCode == nil {
			message = "Docker exited without an exit code"
		} else {
			message = fmt.Sprintf("Docker exited with code %d", *p.ExitCode)
		}
	}
	return errors.New(message)
}
func (b *backend) raw(ctx context.Context, token string, args []string, env map[string]*string, tail bool, budget int) (sdk.ProcessResult, error) {
	timeout := 30000
	if deadline, ok := ctx.Deadline(); ok {
		timeout = int(time.Until(deadline).Milliseconds())
		if timeout < 100 {
			return sdk.ProcessResult{}, context.DeadlineExceeded
		}
		if timeout > 30000 {
			timeout = 30000
		}
	}
	p := sdk.ProcessRequest{Executable: "docker", Args: args, Env: env, TimeoutMS: timeout, MaxOutputBytes: budget}
	if tail {
		p.Output = "tail"
		p.MaxOutputBytes = logBytes
	}
	return b.runtime.Run(ctx, token, p)
}
func (b *backend) negotiate(ctx context.Context, token, contextName string, s *session) (map[string]*string, error) {
	if cached, ok := s.env[contextName]; ok && time.Now().Before(cached.expires) {
		return cached.value, nil
	}
	env := map[string]*string{"DOCKER_API_VERSION": nil}
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		args := []string{}
		if contextName != "" {
			args = append(args, "--context", contextName)
		}
		args = append(args, "info", "--format", `{"ID":{{json .ID}}}`)
		probe, cancel := context.WithTimeout(ctx, 3*time.Second)
		out, err := b.raw(probe, token, args, env, false, 4096)
		cancel()
		if err != nil {
			return nil, err
		}
		if out.ExitCode != nil && *out.ExitCode == 0 {
			var info struct{ ID string }
			if err = json.Unmarshal([]byte(out.Stdout), &info); err != nil {
				return nil, err
			}
			if info.ID != "" {
				s.env[contextName] = environment{env, time.Now().Add(15 * time.Second)}
				return env, nil
			}
			return nil, errors.New("Invalid Docker engine identity")
		}
		match := apiVersion.FindStringSubmatch(out.Stderr)
		if !apiMismatch.MatchString(out.Stderr) || len(match) != 3 || seen[match[2]] {
			return nil, exitError(out)
		}
		version := match[2]
		seen[version] = true
		env = map[string]*string{"DOCKER_API_VERSION": &version}
	}
	return nil, errors.New("Docker client and server have no usable API version")
}
func (b *backend) run(ctx context.Context, token string, args []string, s *session, tail bool) (sdk.ProcessResult, error) {
	contextName := ""
	for i, arg := range args {
		if arg == "--context" && i+1 < len(args) {
			contextName = args[i+1]
			break
		}
	}
	local := len(args) > 0 && (args[0] == "context" || args[0] == "--version")
	env := map[string]*string{"DOCKER_API_VERSION": nil}
	var err error
	if !local {
		env, err = b.negotiate(ctx, token, contextName, s)
		if err != nil {
			return sdk.ProcessResult{}, err
		}
	}
	out, err := b.raw(ctx, token, args, env, tail, 0)
	if err == nil && apiMismatch.MatchString(out.Stderr) {
		delete(s.env, contextName)
		read, write := false, false
		for _, a := range args {
			if a == "ps" || a == "info" || a == "inspect" || a == "ls" || a == "logs" {
				read = true
			}
			if a == "start" || a == "stop" || a == "restart" || a == "rm" || a == "exec" {
				write = true
			}
		}
		if !local && read && !write {
			env, err = b.negotiate(ctx, token, contextName, s)
			if err == nil {
				out, err = b.raw(ctx, token, args, env, tail, 0)
			}
		}
	}
	return out, err
}
func (b *backend) execute(ctx context.Context, token string, args []string, s *session) (string, error) {
	out, err := b.run(ctx, token, args, s, false)
	if err == nil {
		err = exitError(out)
	}
	return out.Stdout, err
}
func (b *backend) verify(ctx context.Context, token string, r request, s *session) error {
	out, err := b.execute(ctx, token, []string{"--context", r.Context, "info", "--format", "{{.ID}}"}, s)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != r.Engine {
		return errors.New("Docker engine changed. Reconnect before continuing.")
	}
	s.verified[r.Context+"\x00"+r.Engine] = time.Now().Add(15 * time.Second)
	return nil
}
func (b *backend) Command(ctx context.Context, scope sdk.SessionInfo, command string, raw json.RawMessage) (any, error) {
	s := b.state(scope.Token)
	s.op.Lock()
	defer s.op.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var out any
	var err error
	if command == "docker.logs" {
		out, err = b.logs(ctx, scope.Token, s, raw)
	} else if command == "docker.request" {
		var r request
		r, err = decodeRequest(raw)
		if err == nil {
			out, err = b.operation(ctx, scope.Token, r, s)
		}
	} else {
		err = errors.New("Invalid Docker command")
	}
	if err != nil {
		var r struct {
			Operation string `json:"operation"`
		}
		_ = json.Unmarshal(raw, &r)
		return map[string]any{"error": err.Error(), "operation": r.Operation}, nil
	}
	return out, nil
}
func decodeLines(text string) ([]map[string]any, error) {
	out := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}
func (b *backend) operation(ctx context.Context, token string, r request, s *session) (any, error) {
	if r.Operation == "connect" {
		s.env = map[string]environment{}
		name, err := b.execute(ctx, token, []string{"context", "show"}, s)
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if !contextName.MatchString(name) {
			return nil, errors.New("Invalid Docker context")
		}
		text, err := b.execute(ctx, token, []string{"--context", name, "info", "--format", infoProjection}, s)
		if err != nil {
			return nil, err
		}
		var info struct{ ID, Name, ServerVersion string }
		if err = json.Unmarshal([]byte(text), &info); err != nil || info.ID == "" {
			return nil, errors.New("Invalid Docker engine identity")
		}
		text, err = b.execute(ctx, token, []string{"context", "inspect", name, "--format", "{{json .Endpoints.docker.Host}}"}, s)
		if err != nil {
			return nil, err
		}
		var endpoint string
		if err = json.Unmarshal([]byte(text), &endpoint); err != nil {
			return nil, err
		}
		s.verified[name+"\x00"+info.ID] = time.Now().Add(15 * time.Second)
		return map[string]any{"data": map[string]any{"context": name, "engine": info.ID, "name": info.Name, "version": info.ServerVersion, "endpoint": endpoint}}, nil
	}
	if mutation(r.Operation) || !time.Now().Before(s.verified[r.Context+"\x00"+r.Engine]) {
		if err := b.verify(ctx, token, r, s); err != nil {
			return nil, err
		}
	}
	if r.Operation == "exec" {
		env, err := b.negotiate(ctx, token, r.Context, s)
		if err != nil {
			return nil, err
		}
		var result any
		title := r.ID
		if len(title) > 12 {
			title = title[:12]
		}
		err = b.runtime.Capability(ctx, "terminal.create", map[string]any{"token": token, "request": sdk.ProcessRequest{Executable: "docker", Args: arguments(r), Env: env, TimeoutMS: 30000, Title: "Docker · " + title, Checks: []sdk.ProcessCheck{{Args: []string{"--context", r.Context, "info", "--format", "{{.ID}}"}, Stdout: r.Engine}}}}, &result)
		return result, err
	}
	out, err := b.run(ctx, token, arguments(r), s, r.Operation == "logs")
	if err == nil {
		err = exitError(out)
	}
	if err != nil {
		return nil, err
	}
	var data any = out.Stdout
	switch r.Operation {
	case "logs":
		text, truncated := clipLogs(out.Stdout+out.Stderr, out.Truncated)
		data = map[string]any{"text": text, "truncated": truncated}
	case "list":
		data, err = decodeLines(out.Stdout)
	case "inspect":
		var c struct {
			ID              string `json:"Id"`
			Name, Created   string
			State           any
			Config          struct{ Image string }
			HostConfig      struct{ RestartPolicy any }
			Mounts          any
			NetworkSettings struct {
				Ports    any
				Networks map[string]any
			}
		}
		err = json.Unmarshal([]byte(out.Stdout), &c)
		networks := []string{}
		for name := range c.NetworkSettings.Networks {
			networks = append(networks, name)
		}
		data = map[string]any{"Id": c.ID, "Name": c.Name, "Created": c.Created, "State": c.State, "Image": c.Config.Image, "RestartPolicy": c.HostConfig.RestartPolicy, "Ports": c.NetworkSettings.Ports, "Mounts": c.Mounts, "Networks": networks}
	case "images.list":
		data, err = b.images(ctx, token, r, s, out.Stdout)
	case "images.inspect":
		var image struct {
			ID                    string `json:"Id"`
			RepoTags, RepoDigests []string
			Created               string
			Size                  int64
			Os, Architecture      string
			LayersCount           int
			Config                struct{ Entrypoint, Cmd []string }
		}
		err = json.Unmarshal([]byte(out.Stdout), &image)
		data = map[string]any{"Id": image.ID, "RepoTags": nonNil(image.RepoTags), "RepoDigests": nonNil(image.RepoDigests), "Created": image.Created, "Size": image.Size, "Os": image.Os, "Architecture": image.Architecture, "Layers": image.LayersCount, "Entrypoint": nonNil(image.Config.Entrypoint), "Cmd": nonNil(image.Config.Cmd)}
	}
	return map[string]any{"data": data}, err
}
func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
func (b *backend) images(ctx context.Context, token string, r request, s *session, text string) (any, error) {
	rows, err := decodeLines(text)
	if err != nil {
		return nil, err
	}
	byID := map[string]map[string]any{}
	order := []string{}
	for _, row := range rows {
		id, _ := row["ID"].(string)
		if !imageID.MatchString(id) {
			return nil, errors.New("Invalid image ID from Docker")
		}
		item := byID[id]
		if item == nil {
			item = map[string]any{"ID": id, "RepoTags": []string{}, "CreatedAt": row["CreatedAt"], "Size": row["Size"]}
			byID[id] = item
			order = append(order, id)
		}
		repo, _ := row["Repository"].(string)
		tag, _ := row["Tag"].(string)
		if repo != "" && repo != "<none>" && tag != "" && tag != "<none>" {
			value := repo + ":" + tag
			tags := item["RepoTags"].([]string)
			found := false
			for _, t := range tags {
				found = found || t == value
			}
			if !found {
				item["RepoTags"] = append(tags, value)
			}
		}
	}
	if len(order) == 0 {
		return []any{}, nil
	}
	counts, err := b.imageUsage(ctx, token, r.Context, s)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, id := range order {
		item := byID[id]
		item["Containers"] = counts[strings.TrimPrefix(id, "sha256:")]
		out = append(out, item)
	}
	return out, nil
}
func (b *backend) imageUsage(ctx context.Context, token, contextName string, s *session) (map[string]int, error) {
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		var counts map[string]int
		counts, err = b.imageUsageOnce(ctx, token, contextName, s)
		if err == nil {
			return counts, nil
		}
		if !regexp.MustCompile(`(?i)No such (object|container)`).MatchString(err.Error()) {
			break
		}
	}
	return nil, err
}
func (b *backend) imageUsageOnce(ctx context.Context, token, contextName string, s *session) (map[string]int, error) {
	out, err := b.execute(ctx, token, []string{"--context", contextName, "ps", "-a", "--quiet", "--no-trunc"}, s)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, id := range strings.Fields(out) {
		if !immutableID.MatchString(id) {
			return nil, errors.New("Invalid container ID from Docker")
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	counts := map[string]int{}
	for start := 0; start < len(ids); start += 100 {
		end := start + 100
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		args := append([]string{"--context", contextName, "inspect", "--type", "container", "--format", `{"ID":{{json .Id}},"ImageID":{{json .Image}}}`}, batch...)
		text, err := b.execute(ctx, token, args, s)
		if err != nil {
			return nil, err
		}
		rows, err := decodeLines(text)
		if err != nil {
			return nil, err
		}
		expected := map[string]bool{}
		for _, id := range batch {
			expected[id] = true
		}
		for _, row := range rows {
			id, _ := row["ID"].(string)
			image, _ := row["ImageID"].(string)
			if !expected[id] || !immutableID.MatchString(strings.TrimPrefix(image, "sha256:")) {
				return nil, errors.New("Invalid container image reference from Docker")
			}
			delete(expected, id)
			counts[strings.TrimPrefix(image, "sha256:")]++
		}
		if len(expected) != 0 {
			return nil, errors.New("Incomplete container image references from Docker")
		}
	}
	return counts, nil
}
func clipLogs(text string, truncated bool) (string, bool) {
	if len(text) > logBytes {
		text = text[len(text)-logBytes:]
		for len(text) > 0 && !utf8.RuneStart(text[0]) {
			text = text[1:]
		}
		truncated = true
	}
	newline := strings.HasSuffix(text, "\n")
	lines := strings.Split(text, "\n")
	if newline {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > logLines {
		lines = lines[len(lines)-logLines:]
		text = strings.Join(lines, "\n")
		if newline {
			text += "\n"
		}
		truncated = true
	}
	return text, truncated
}
func (b *backend) publish(ctx context.Context, token string, s *session, views map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	payload := map[string]any{"state": "ready", "items": []any{}}
	if views != nil {
		payload["views"] = views
	}
	if log := s.log; log != nil {
		snapshot := map[string]any{"subscription": log.subscription, "id": log.id, "text": log.text, "truncated": log.truncated, "ended": log.ended}
		if log.failure != "" {
			snapshot["error"] = log.failure
		}
		payload["data"] = map[string]any{"dockerLogs": snapshot}
	}
	return b.runtime.Publish(ctx, token, payload)
}
func (b *backend) logs(ctx context.Context, token string, s *session, raw json.RawMessage) (any, error) {
	var p struct{ Action, Subscription, ID, Context, Engine string }
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil || !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`).MatchString(p.Subscription) || (p.Action != "start" && p.Action != "stop") {
		return nil, errors.New("Invalid log subscription")
	}
	s.mu.Lock()
	previous := s.log
	if p.Action == "stop" && (previous == nil || previous.subscription != p.Subscription) {
		s.mu.Unlock()
		return nil, nil
	}
	s.log = nil
	s.mu.Unlock()
	if previous != nil {
		b.mu.Lock()
		delete(b.streams, previous.stream)
		b.mu.Unlock()
		_ = b.runtime.Capability(ctx, "process.stream.stop", map[string]string{"token": token, "id": previous.stream}, nil)
	}
	if p.Action == "stop" {
		return nil, nil
	}
	r, err := decodeRequest(sdk.JSON(request{Operation: "logs", ID: p.ID, Context: p.Context, Engine: p.Engine}))
	if err != nil {
		return nil, err
	}
	if err = b.verify(ctx, token, r, s); err != nil {
		return nil, err
	}
	env, err := b.negotiate(ctx, token, r.Context, s)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.sequence++
	id := fmt.Sprintf("docker-log-%d", b.sequence)
	b.streams[id] = token
	b.mu.Unlock()
	log := &logState{subscription: p.Subscription, id: p.ID, stream: id}
	s.mu.Lock()
	s.log = log
	s.mu.Unlock()
	args := arguments(r)
	args = append(args[:len(args)-1], "--follow", r.ID)
	err = b.runtime.Capability(ctx, "process.stream.start", map[string]any{"token": token, "id": id, "request": sdk.ProcessRequest{Executable: "docker", Args: args, Env: env, MaxOutputBytes: logBytes}}, nil)
	if err != nil {
		b.mu.Lock()
		delete(b.streams, id)
		b.mu.Unlock()
		s.mu.Lock()
		if s.log == log {
			s.log = nil
		}
		s.mu.Unlock()
	}
	return nil, err
}
func (b *backend) Stream(ctx context.Context, e sdk.StreamEvent) error {
	b.mu.Lock()
	token := b.streams[e.ID]
	s := b.sessions[token]
	b.mu.Unlock()
	if token == "" || s == nil {
		return nil
	}
	defer func() {
		_ = b.runtime.Capability(ctx, "process.stream.ack", map[string]string{"token": token, "id": e.ID}, nil)
	}()
	s.mu.Lock()
	log := s.log
	if log == nil || log.stream != e.ID {
		s.mu.Unlock()
		return nil
	}
	if e.Truncated {
		log.text = ""
		log.truncated = true
	}
	log.text, log.truncated = clipLogs(log.text+e.Text, log.truncated)
	log.ended = e.Done
	log.failure = e.Error
	if e.Done && e.ExitCode != nil && *e.ExitCode != 0 && log.failure == "" {
		log.failure = fmt.Sprintf("Docker exited with code %d", *e.ExitCode)
	}
	s.mu.Unlock()
	err := b.publish(ctx, token, s, nil)
	if e.Done {
		b.mu.Lock()
		delete(b.streams, e.ID)
		b.mu.Unlock()
	}
	return err
}
func (b *backend) Refresh(ctx context.Context, scope sdk.SessionInfo) error {
	s := b.state(scope.Token)
	s.op.Lock()
	defer s.op.Unlock()
	if time.Now().After(s.availableUntil) {
		visible, available, reason := true, false, "unknown"
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		client, err := b.run(probe, scope.Token, []string{"--version"}, s, false)
		cancel()
		message := client.Stderr
		if err != nil {
			message = err.Error()
		}
		missing := regexp.MustCompile(`(?i)docker.*(not found|no such file|not recognized)|Executable not found: docker`).MatchString(message)
		if err != nil || exitError(client) != nil {
			if missing {
				visible = false
				reason = "missing"
			} else if regexp.MustCompile(`(?i)permission denied|access denied`).MatchString(message) {
				reason = "availabilityPermission"
			} else {
				reason = "availabilityUnknown"
			}
		} else {
			probe, cancel = context.WithTimeout(ctx, 3*time.Second)
			engine, e := b.run(probe, scope.Token, []string{"info", "--format", "{{.ID}}"}, s, false)
			cancel()
			available = e == nil && exitError(engine) == nil && strings.TrimSpace(engine.Stdout) != ""
			reason = "availabilityDaemon"
			if regexp.MustCompile(`(?i)permission denied|access denied`).MatchString(engine.Stderr) {
				reason = "availabilityPermission"
			}
		}
		s.availabilityReason = reason
		s.availability = map[string]any{"visible": visible, "available": available}
		if !available {
			s.availability["message"] = b.runtime.T(reason, nil)
		}
		s.availableUntil = time.Now().Add(15 * time.Second)
	}
	return b.publish(ctx, scope.Token, s, map[string]any{"docker.containers": s.availability})
}
func (b *backend) LocaleChanged(ctx context.Context, sessions []sdk.SessionInfo) {
	for _, scope := range sessions {
		b.mu.Lock()
		s := b.sessions[scope.Token]
		b.mu.Unlock()
		if s == nil {
			continue
		}
		s.op.Lock()
		if s.availability != nil {
			if s.availability["available"] != true {
				s.availability["message"] = b.runtime.T(s.availabilityReason, nil)
			}
			_ = b.publish(ctx, scope.Token, s, map[string]any{"docker.containers": s.availability})
		}
		s.op.Unlock()
	}
}
