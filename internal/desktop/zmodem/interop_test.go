package zmodem

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
)

func TestElectronZmodemWireInteroperability(t *testing.T) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR for Electron ZMODEM wire interop")
	}
	type vector struct {
		Kind    string    `json:"kind"`
		Name    string    `json:"name,omitempty"`
		Type    FrameType `json:"type"`
		Data    []int     `json:"data,omitempty"`
		Format  string    `json:"format"`
		End     string    `json:"end,omitempty"`
		EndCode byte      `json:"endCode"`
		Payload string    `json:"payload,omitempty"`
		Wire    string    `json:"wire"`
	}
	vectors := []vector{}
	formats := []struct {
		name  string
		value Encoding
	}{{"hex", Hex}, {"16", Binary16}, {"32", Binary32}}
	names := []struct {
		name string
		kind FrameType
	}{{"ZRQINIT", ZRQINIT}, {"ZRINIT", ZRINIT}, {"ZACK", ZACK}, {"ZFIN", ZFIN}, {"ZRPOS", ZRPOS}, {"ZDATA", ZDATA}, {"ZEOF", ZEOF}}
	for _, format := range formats {
		for _, name := range names {
			header := OffsetHeader(name.kind, 0xff181100)
			wire, err := EncodeHeader(header, format.value)
			if err != nil {
				t.Fatal(err)
			}
			vectors = append(vectors, vector{Kind: "header", Name: name.name, Type: name.kind, Data: []int{0, 17, 24, 255}, Format: format.name, Wire: base64.StdEncoding.EncodeToString(wire)})
		}
	}
	payload := make([]byte, 1024)
	for i := range payload {
		payload[i] = byte(i)
	}
	ends := []struct {
		name  string
		value byte
	}{{"end_no_ack", ZCRCE}, {"no_end_no_ack", ZCRCG}, {"no_end_ack", ZCRCQ}, {"end_ack", ZCRCW}}
	for _, payload := range [][]byte{nil, payload} {
		for _, format := range formats[1:] {
			for _, end := range ends {
				wire, err := EncodeSubpacket(payload, end.value, format.value)
				if err != nil {
					t.Fatal(err)
				}
				vectors = append(vectors, vector{Kind: "data", Format: format.name, End: end.name, EndCode: end.value, Payload: base64.StdEncoding.EncodeToString(payload), Wire: base64.StdEncoding.EncodeToString(wire)})
			}
		}
	}

	script := `
const fs=require('node:fs'),path=require('node:path');
const root=process.argv[1];const load=name=>require(path.join(root,'node_modules/zmodem.js-ex/src',name));
const Header=load('zheader.js').Header,Subpacket=load('zsubpacket.js').Subpacket,ZDLE=load('zdle.js').ZDLE;
const requests=JSON.parse(fs.readFileSync(0,'utf8'));
const result=requests.map(v=>{
 const encoder=new ZDLE({escape_ctrl_chars:true});let bytes;
 if(v.kind==='header'){
  const header=v.name==='ZRINIT'?Header.build(v.name,[]):Header.build(v.name,0);header._bytes4=v.data;
  bytes=v.format==='hex'?header.to_hex():v.format==='32'?header.to_binary32(encoder):header.to_binary16(encoder);
  const parsed=Header.parse(Array.from(Buffer.from(v.wire,'base64')));
  if(!parsed || parsed[0].TYPENUM!==v.type || JSON.stringify(parsed[0]._bytes4)!==JSON.stringify(v.data))throw Error('Electron rejected Go header');
 }else{
  const payload=Array.from(Buffer.from(v.payload||'','base64')),packet=Subpacket.build(payload,v.end);
  bytes=v.format==='32'?packet.encode32(encoder):packet.encode16(encoder);
  const parsed=(v.format==='32'?Subpacket.parse32:Subpacket.parse16)(Array.from(Buffer.from(v.wire,'base64')));
  if(!parsed || parsed._frameend_num!==v.endCode || !Buffer.from(parsed.get_payload()).equals(Buffer.from(payload)))throw Error('Electron rejected Go data packet');
 }
 return Buffer.from(bytes).toString('base64');
});process.stdout.write(JSON.stringify(result));
`
	input, err := json.Marshal(vectors)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("node", "-e", script, reference)
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Electron wire interop: %v\n%s", err, output)
	}
	var peers []string
	if err = json.Unmarshal(output, &peers); err != nil {
		t.Fatal(err)
	}
	if len(peers) != len(vectors) {
		t.Fatal("missing peer vectors")
	}
	for i, peer := range peers {
		if peer != vectors[i].Wire {
			t.Fatalf("wire output differs from Electron at %d (%s/%s)", i, vectors[i].Kind, vectors[i].Format)
		}
		wire, err := base64.StdEncoding.DecodeString(peer)
		if err != nil {
			t.Fatal(err)
		}
		var format Encoding
		switch vectors[i].Format {
		case "hex":
			format = Hex
		case "16":
			format = Binary16
		case "32":
			format = Binary32
		}
		if vectors[i].Kind == "header" {
			if _, used, err := DecodeHeader(wire); err != nil || used != len(wire) {
				t.Fatalf("Go rejected Electron header %d: %v", i, err)
			}
		} else {
			if packet, used, err := DecodeSubpacket(wire, format); err != nil || used != len(wire) || !bytes.Equal(packet.Payload, mustDecodePayload(t, vectors[i].Payload)) {
				t.Fatalf("Go rejected Electron packet %d: %v", i, err)
			}
		}
	}
}

func mustDecodePayload(t *testing.T, value string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
