package service

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestRemoteTextLanguageRecognizesCommonNamesAndExtensions(t *testing.T) {
	cases := map[string]string{
		"/etc/nginx.conf": "nginx",
		"/srv/Dockerfile": "dockerfile",
		"/srv/Makefile":   "makefile",
		"/srv/.env.local": "shell",
		"/srv/main.tsx":   "typescript",
		"/srv/notes":      "plaintext",
	}
	for path, want := range cases {
		if got := remoteTextLanguage(path); got != want {
			t.Errorf("remoteTextLanguage(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestValidatePreviewImageAcceptsPNGAndRejectsText(t *testing.T) {
	imageData := bytes.Buffer{}
	canvas := image.NewRGBA(image.Rect(0, 0, 3, 2))
	canvas.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&imageData, canvas); err != nil {
		t.Fatal(err)
	}
	mediaType, width, height, err := validatePreviewImage(imageData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if mediaType != "image/png" || width != 3 || height != 2 {
		t.Fatalf("preview = (%q, %d, %d), want PNG 3x2", mediaType, width, height)
	}
	if _, _, _, err := validatePreviewImage([]byte("not an image")); err == nil {
		t.Fatal("plain text was accepted as image preview")
	}
}

func TestWebPDimensions(t *testing.T) {
	data := make([]byte, 30)
	copy(data, "RIFF\x16\x00\x00\x00WEBPVP8X")
	data[24], data[25], data[26] = 4, 0, 0 // stored as width minus one
	data[27], data[28], data[29] = 2, 0, 0
	mediaType, width, height, err := validatePreviewImage(data)
	if err != nil {
		t.Fatal(err)
	}
	if mediaType != "image/webp" || width != 5 || height != 3 {
		t.Fatalf("preview = (%q, %d, %d), want WebP 5x3", mediaType, width, height)
	}
}

func TestRemoteTextPayloadGuards(t *testing.T) {
	if bytes.IndexByte([]byte("plain text"), 0) >= 0 {
		t.Fatal("plain text unexpectedly contains NUL")
	}
	if len(base64.StdEncoding.EncodeToString(make([]byte, maxRemoteTextFileBytes))) == 0 {
		t.Fatal("expected encoded text payload")
	}
}
