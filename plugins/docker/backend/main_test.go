package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	sdk "ssh-connect/packages/plugin-go"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

type hostFunc func(string, json.RawMessage) (any, error)

func (f hostFunc) Call(_ context.Context, m string, a any, out any) error {
	raw, _ := json.Marshal(a)
	v, e := f(m, raw)
	if e != nil {
		return e
	}
	if out != nil {
		raw, _ = json.Marshal(v)
		return json.Unmarshal(raw, out)
	}
	return nil
}
func (hostFunc) Open(context.Context, string, any) (net.Conn, error) {
	return nil, fmt.Errorf("unused")
}
func result(stdout, stderr string, code int) sdk.ProcessResult {
	return sdk.ProcessResult{Stdout: stdout, Stderr: stderr, ExitCode: &code}
}
func testBackend(f func(sdk.ProcessRequest) (sdk.ProcessResult, error)) *backend {
	return newBackend(&sdk.SessionRuntime{Host: hostFunc(func(method string, raw json.RawMessage) (any, error) {
		if method != "process.execute" {
			return nil, fmt.Errorf("unexpected %s", method)
		}
		var p struct{ Request sdk.ProcessRequest }
		json.Unmarshal(raw, &p)
		return f(p.Request)
	})})
}
func TestNegotiationAndMutationNeverRetried(t *testing.T) {
	calls, mutations := 0, 0
	b := testBackend(func(p sdk.ProcessRequest) (sdk.ProcessResult, error) {
		calls++
		if p.Args[2] == "info" {
			if calls == 1 {
				return result("", "client version 1.50 is too new. Maximum supported API version is 1.44", 1), nil
			}
			if p.Env["DOCKER_API_VERSION"] == nil || *p.Env["DOCKER_API_VERSION"] != "1.44" {
				t.Fatal("missing negotiated version")
			}
			return result(`{"ID":"engine"}`, "", 0), nil
		}
		mutations++
		return result("", "client version 1.44 is too old. Minimum supported API version is 1.45", 1), nil
	})
	s := b.state("token")
	out, e := b.run(context.Background(), "token", []string{"--context", "default", "start", "abcdefabcdef"}, s, false)
	if e != nil || exitError(out) == nil || calls != 3 || mutations != 1 {
		t.Fatalf("calls=%d mutations=%d result=%+v err=%v", calls, mutations, out, e)
	}
}
func TestMutationChecksEngineBeforeExecution(t *testing.T) {
	calls := 0
	b := testBackend(func(p sdk.ProcessRequest) (sdk.ProcessResult, error) {
		calls++
		if p.Args[2] != "info" {
			t.Fatal("mutation reached changed engine")
		}
		return result(`{"ID":"different"}`, "", 0), nil
	})
	s := b.state("t")
	s.verified["default\x00original"] = time.Now().Add(time.Minute)
	_, e := b.operation(context.Background(), "t", request{Operation: "stop", ID: "abcdefabcdef", Context: "default", Engine: "original"}, s)
	if e == nil || !strings.Contains(e.Error(), "engine changed") || calls == 0 {
		t.Fatal(e, calls)
	}
}
func TestNegotiationRejectsMissingIdentity(t *testing.T) {
	b := testBackend(func(sdk.ProcessRequest) (sdk.ProcessResult, error) { return result(`{}`, "", 0), nil })
	if _, e := b.negotiate(context.Background(), "t", "default", b.state("t")); e == nil {
		t.Fatal("accepted missing identity")
	}
}
func TestLogBufferUTF8AndGap(t *testing.T) {
	s, truncated := clipLogs(strings.Repeat("你", 10000), false)
	if !truncated || len(s) > logBytes || !utf8.ValidString(s) {
		t.Fatal("invalid UTF-8 bounded tail")
	}
	s, truncated = clipLogs(strings.Repeat("line\n", 500), false)
	if !truncated || strings.Count(s, "\n") > logLines {
		t.Fatal("unbounded lines")
	}
}
func TestLogEventsPublishThenAckAndRelease(t *testing.T) {
	var methods []string
	var stopped bool
	r := &sdk.SessionRuntime{Host: hostFunc(func(m string, raw json.RawMessage) (any, error) {
		methods = append(methods, m)
		if m == "process.stream.stop" {
			stopped = true
		}
		return nil, nil
	})}
	b := newBackend(r)
	s := b.state("t")
	s.log = &logState{subscription: "sub", stream: "stream", id: "abcdefabcdef", text: "old\n"}
	b.streams["stream"] = "t"
	if e := b.Stream(context.Background(), sdk.StreamEvent{ID: "stream", Text: "新\n", Truncated: true}); e != nil {
		t.Fatal(e)
	}
	if s.log.text != "新\n" || !s.log.truncated || strings.Join(methods, ",") != "status.publish,process.stream.ack" {
		t.Fatal(methods, s.log)
	}
	b.Release("t")
	if b.Tasks() != 0 || !stopped {
		t.Fatal("stream survived scope release")
	}
}
func TestImageUsageIncludesStoppedContainersAndRetriesSnapshot(t *testing.T) {
	a, c, img := strings.Repeat("a", 64), strings.Repeat("c", 64), strings.Repeat("b", 64)
	attempts := 0
	b := testBackend(func(p sdk.ProcessRequest) (sdk.ProcessResult, error) {
		switch p.Args[2] {
		case "info":
			return result(`{"ID":"engine"}`, "", 0), nil
		case "ps":
			if !strings.Contains(strings.Join(p.Args, " "), "-a --quiet --no-trunc") {
				t.Fatal("must include stopped containers")
			}
			return result(a+"\n"+c, "", 0), nil
		case "inspect":
			attempts++
			if attempts == 1 {
				return result("", "No such container: "+a, 1), nil
			}
			return result(fmt.Sprintf("{\"ID\":%q,\"ImageID\":%q}\n{\"ID\":%q,\"ImageID\":%q}", a, "sha256:"+img, c, "sha256:"+img), "", 0), nil
		}
		return sdk.ProcessResult{}, fmt.Errorf("unexpected %v", p.Args)
	})
	counts, e := b.imageUsage(context.Background(), "t", "default", b.state("t"))
	if e != nil || counts[img] != 2 || attempts != 2 {
		t.Fatal(counts, e, attempts)
	}
}
