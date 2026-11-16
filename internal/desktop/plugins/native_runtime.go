package plugins

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
)

const WorkerArgument = "--aterm-plugin-worker"
const workerMessageLimit = 2 << 20

//go:embed native_sdk.js
var nativeSDK string

type workerPacket struct {
	Control  *RuntimeControl `json:"control,omitempty"`
	Method   string          `json:"method,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
	Response *workerReply    `json:"response,omitempty"`
}

type workerReply struct {
	OK    bool `json:"ok"`
	Value any  `json:"value,omitempty"`
}

type nativeWorker struct {
	cancel context.CancelFunc
	out    chan workerPacket
}

type nativeTransport struct {
	mu      sync.Mutex
	workers map[string]*nativeWorker
	handle  func(context.Context, string, string, json.RawMessage) (any, error)
}

func NewNativeRuntime(request RuntimeRequest) *RuntimeHost {
	transport := &nativeTransport{workers: make(map[string]*nativeWorker)}
	host := NewRuntimeHost(transport.send, request)
	transport.handle = host.Handle
	return host
}

func (t *nativeTransport) send(control RuntimeControl) {
	id := control.InstanceID
	t.mu.Lock()
	if control.Kind == "start" {
		ctx, cancel := context.WithCancel(context.Background())
		worker := &nativeWorker{cancel: cancel, out: make(chan workerPacket, 64)}
		t.workers[id] = worker
		worker.out <- workerPacket{Control: &control}
		t.mu.Unlock()
		go t.run(ctx, id, worker)
		return
	}
	worker := t.workers[id]
	if worker != nil {
		if control.Kind == "stop" {
			delete(t.workers, id)
			worker.cancel()
		} else {
			select {
			case worker.out <- workerPacket{Control: &control}:
			default:
				worker.cancel()
			}
		}
	}
	t.mu.Unlock()
}

func (t *nativeTransport) run(ctx context.Context, id string, worker *nativeWorker) {
	defer func() {
		worker.cancel()
		_, _ = t.handle(context.Background(), id, "runtime.fault", json.RawMessage("{}"))
	}()
	executable, err := os.Executable()
	if err != nil {
		return
	}
	command := exec.CommandContext(ctx, executable, WorkerArgument)
	command.Env = workerEnvironment()
	configureWorkerCommand(command)
	input, err := command.StdinPipe()
	if err != nil {
		return
	}
	defer input.Close()
	output, err := command.StdoutPipe()
	if err != nil {
		return
	}
	defer output.Close()
	if command.Start() != nil {
		return
	}
	defer command.Wait()
	defer worker.cancel()
	go func() {
		encoder := json.NewEncoder(input)
		for {
			select {
			case packet := <-worker.out:
				if encoder.Encode(packet) != nil {
					worker.cancel()
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	scanner := workerScanner(output)
	for scanner.Scan() {
		var packet workerPacket
		if json.Unmarshal(scanner.Bytes(), &packet) != nil || packet.Method == "" {
			return
		}
		value, err := t.handle(ctx, id, packet.Method, packet.Params)
		select {
		case worker.out <- workerPacket{Response: &workerReply{OK: err == nil, Value: value}}:
		case <-ctx.Done():
			return
		}
	}
}

func workerScanner(input io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), workerMessageLimit)
	return scanner
}

// ServeWorker evaluates JS in a child process. Its only host binding is the
// capability broker. Goja receives no filesystem, environment or network API.
func ServeWorker(input io.Reader, output io.Writer) error {
	scanner := workerScanner(input)
	encoder := json.NewEncoder(output)
	next := func() (workerPacket, error) {
		var packet workerPacket
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return packet, err
			}
			return packet, io.EOF
		}
		return packet, json.Unmarshal(scanner.Bytes(), &packet)
	}
	start, err := next()
	if err != nil {
		return err
	}
	if start.Control == nil || start.Control.Kind != "start" {
		return errors.New("missing worker start")
	}
	debug.SetMemoryLimit(64 << 20)
	vm := goja.New()
	vm.SetMaxCallStackSize(512)
	var queued []*RuntimeControl
	request := func(method string, params string) string {
		if !json.Valid([]byte(params)) || len(params) > 262144 {
			return `{"ok":false}`
		}
		if err := encoder.Encode(workerPacket{Method: method, Params: json.RawMessage(params)}); err != nil {
			panic(vm.NewGoError(err))
		}
		for {
			packet, err := next()
			if err != nil {
				panic(vm.NewGoError(err))
			}
			if packet.Response != nil {
				reply, err := json.Marshal(packet.Response)
				if err != nil {
					panic(vm.NewGoError(err))
				}
				return string(reply)
			}
			if packet.Control == nil || len(queued) >= 64 {
				panic(vm.NewGoError(errors.New("invalid worker response")))
			}
			queued = append(queued, packet.Control)
		}
	}
	config, err := json.Marshal(start.Control)
	if err != nil {
		return err
	}
	if err := vm.Set("__request", request); err != nil {
		return err
	}
	if err := vm.Set("__plural", cardinalCategory); err != nil {
		return err
	}
	if err := vm.Set("__config", string(config)); err != nil {
		return err
	}
	bounded := func(run func() error) error {
		// Process exit is the final deadline, including time spent in native
		// engine builtins or while awaiting a capability response.
		timer := time.AfterFunc(45*time.Second, func() { os.Exit(124) })
		defer timer.Stop()
		return run()
	}
	var dispatch goja.Callable
	if err := bounded(func() error {
		value, err := vm.RunString(nativeSDK)
		if err != nil {
			return err
		}
		var ok bool
		dispatch, ok = goja.AssertFunction(value)
		if !ok {
			return errors.New("invalid plugin SDK bootstrap")
		}
		_, err = vm.RunString("(function () {\n" + start.Control.Source + "\n})();")
		return err
	}); err != nil {
		return err
	}
	if reply := request("runtime.ready", "{}"); reply != `{"ok":true}` {
		return errors.New("runtime rejected activation")
	}
	for {
		var control *RuntimeControl
		if len(queued) > 0 {
			control = queued[0]
			queued[0] = nil
			queued = queued[1:]
		} else {
			packet, err := next()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			control = packet.Control
		}
		if control == nil {
			return errors.New("invalid worker control")
		}
		data, err := json.Marshal(control)
		if err != nil {
			return err
		}
		if err := bounded(func() error {
			_, err := dispatch(goja.Undefined(), vm.ToValue(string(data)))
			return err
		}); err != nil {
			return err
		}
	}
}

// JS Intl.PluralRules defaults to three fractional digits for cardinal rules.
func cardinalCategory(locale string, count float64) string {
	if math.IsNaN(count) || math.IsInf(count, 0) {
		return "other"
	}
	tag, err := language.Parse(locale)
	if err != nil {
		return "other"
	}
	formatted := strings.TrimRight(strings.TrimRight(strconv.FormatFloat(math.Abs(count), 'f', 3, 64), "0"), ".")
	integer, fraction, _ := strings.Cut(formatted, ".")
	digits := []byte(integer + fraction)
	for i := range digits {
		digits[i] -= '0'
	}
	form := plural.Cardinal.MatchDigits(tag, digits, len(integer), len(fraction))
	return [...]string{"other", "zero", "one", "two", "few", "many"}[form]
}
