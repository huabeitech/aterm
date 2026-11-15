package service

import "testing"

func TestValidateHostInputAcceptsValidHost(t *testing.T) {
	input := HostInput{
		Alias:    "prod-api",
		HostName: "10.0.0.12",
		User:     "ubuntu",
		Port:     22,
	}

	if err := ValidateHostInput(input); err != nil {
		t.Fatalf("ValidateHostInput() error = %v", err)
	}
}

func TestValidateHostInputRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		input HostInput
	}{
		{
			name: "empty alias",
			input: HostInput{
				HostName: "example.com",
				Port:     22,
			},
		},
		{
			name: "alias with whitespace",
			input: HostInput{
				Alias:    "prod api",
				HostName: "example.com",
				Port:     22,
			},
		},
		{
			name: "empty host name",
			input: HostInput{
				Alias: "prod-api",
				Port:  22,
			},
		},
		{
			name: "port below range",
			input: HostInput{
				Alias:    "prod-api",
				HostName: "example.com",
				Port:     -1,
			},
		},
		{
			name: "port above range",
			input: HostInput{
				Alias:    "prod-api",
				HostName: "example.com",
				Port:     65536,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateHostInput(tt.input); err == nil {
				t.Fatal("ValidateHostInput() error = nil, want error")
			}
		})
	}
}

func TestNormalizeHostInput(t *testing.T) {
	input := HostInput{
		Alias:        "  prod-api  ",
		HostName:     "  example.com  ",
		User:         "  root  ",
		IdentityFile: "  ~/.ssh/prod.pem  ",
		ProxyJump:    "  bastion  ",
		Port:         0,
	}

	got := NormalizeHostInput(input)

	if got.Alias != "prod-api" {
		t.Fatalf("Alias = %q, want prod-api", got.Alias)
	}
	if got.HostName != "example.com" {
		t.Fatalf("HostName = %q, want example.com", got.HostName)
	}
	if got.User != "root" {
		t.Fatalf("User = %q, want root", got.User)
	}
	if got.IdentityFile != "~/.ssh/prod.pem" {
		t.Fatalf("IdentityFile = %q, want ~/.ssh/prod.pem", got.IdentityFile)
	}
	if got.ProxyJump != "bastion" {
		t.Fatalf("ProxyJump = %q, want bastion", got.ProxyJump)
	}
	if got.Port != 22 {
		t.Fatalf("Port = %d, want 22", got.Port)
	}
}

func TestValidateDirectoryInputAcceptsValidDirectory(t *testing.T) {
	input := DirectoryInput{
		ParentID: " parent-id ",
		Name:     " Production API ",
	}

	if err := ValidateDirectoryInput(input); err != nil {
		t.Fatalf("ValidateDirectoryInput() error = %v", err)
	}
}

func TestValidateDirectoryInputRejectsInvalidNames(t *testing.T) {
	tests := []struct {
		name  string
		input DirectoryInput
		want  error
	}{
		{
			name:  "empty name",
			input: DirectoryInput{Name: " "},
			want:  ErrDirectoryNameRequired,
		},
		{
			name:  "too long",
			input: DirectoryInput{Name: "123456789012345678901234567890123456789012345678901"},
			want:  ErrDirectoryNameTooLong,
		},
		{
			name:  "path separator",
			input: DirectoryInput{Name: "Production/API"},
			want:  ErrDirectoryNameInvalidCharacters,
		},
		{
			name:  "filesystem special character",
			input: DirectoryInput{Name: "Production*API"},
			want:  ErrDirectoryNameInvalidCharacters,
		},
		{
			name:  "control character",
			input: DirectoryInput{Name: "Production\nAPI"},
			want:  ErrDirectoryNameInvalidCharacters,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateDirectoryInput(tt.input); err != tt.want {
				t.Fatalf("ValidateDirectoryInput() error = %v, want %v", err, tt.want)
			}
		})
	}
}
