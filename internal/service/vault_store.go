package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	desktopkeys "ssh-connect/internal/desktop/keys"
)

const (
	vaultFormatVersion   = 1
	vaultSchemaVersion   = 1
	vaultKDFAlgorithm    = "argon2id"
	vaultCipherAlgorithm = "xchacha20-poly1305"

	vaultMinimumMemoryKiB = 64 * 1024
	vaultMinimumTime      = 3
)

var (
	ErrVaultLocked              = errors.New("vault is locked")
	ErrInvalidVaultPassword     = errors.New("invalid encryption password")
	ErrVaultCorrupted           = errors.New("encrypted vault is corrupted")
	ErrVaultVersionUnsupported  = errors.New("vault version is not supported")
	ErrSecureStorageUnavailable = errors.New("secure system storage is unavailable")
)

// VaultPayload is the complete encrypted sync workspace. Snapshot contains
// host/directory/sort data with device-only private-key paths removed.
// Metadata stays inside the authenticated ciphertext.
type VaultPayload struct {
	SchemaVersion  int                      `json:"schemaVersion"`
	Snapshot       []byte                   `json:"snapshot"`
	Passwords      map[string]string        `json:"passwords"`
	ManagedKeys    *[]desktopkeys.BackupKey `json:"managedKeys,omitempty"`
	SourceDevice   string                   `json:"sourceDevice"`
	HostCount      int                      `json:"hostCount"`
	DirectoryCount int                      `json:"directoryCount"`
	SyncedAt       string                   `json:"syncedAt"`
}

type vaultKDFParameters struct {
	Algorithm   string `json:"algorithm"`
	Version     int    `json:"version"`
	Salt        string `json:"salt"`
	MemoryKiB   uint32 `json:"memoryKiB"`
	Time        uint32 `json:"time"`
	Parallelism uint8  `json:"parallelism"`
}

type vaultCiphertext struct {
	Algorithm  string `json:"algorithm"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type vaultEnvelope struct {
	FormatVersion int                `json:"formatVersion"`
	VaultID       string             `json:"vaultID"`
	KDF           vaultKDFParameters `json:"kdf"`
	WrappedKey    vaultCiphertext    `json:"wrappedKey"`
	Revision      string             `json:"revision"`
	Payload       vaultCiphertext    `json:"payload"`
}

type OpenedVault struct {
	VaultID  string
	Revision string
	Payload  VaultPayload
}

// VaultStore owns the versioned cryptographic format. It deliberately has no
// filesystem or transport behavior, making the same object portable across
// iCloud Drive, ordinary synced folders, and future transports.
type VaultStore struct {
	memoryKiB   uint32
	time        uint32
	parallelism uint8
}

func NewVaultStore() *VaultStore {
	return &VaultStore{
		memoryKiB:   vaultMinimumMemoryKiB,
		time:        vaultMinimumTime,
		parallelism: 1,
	}
}

func (s *VaultStore) Create(password string, payload VaultPayload) ([]byte, []byte, error) {
	if password == "" {
		return nil, nil, errors.New("encryption password is required")
	}
	vaultKey, err := randomBytes(chacha20poly1305.KeySize)
	if err != nil {
		return nil, nil, err
	}
	salt, err := randomBytes(16)
	if err != nil {
		return nil, nil, err
	}
	envelope := vaultEnvelope{
		FormatVersion: vaultFormatVersion,
		VaultID:       uuid.NewString(),
		KDF: vaultKDFParameters{
			Algorithm: vaultKDFAlgorithm, Version: 1,
			Salt:      base64.RawStdEncoding.EncodeToString(salt),
			MemoryKiB: s.memoryKiB, Time: s.time, Parallelism: s.parallelism,
		},
		Revision: uuid.NewString(),
	}
	wrappingKey := argon2.IDKey([]byte(password), salt, envelope.KDF.Time, envelope.KDF.MemoryKiB, envelope.KDF.Parallelism, chacha20poly1305.KeySize)
	envelope.WrappedKey, err = sealVaultBytes(wrappingKey, vaultKey, vaultKeyAssociatedData(envelope))
	if err != nil {
		return nil, nil, err
	}
	envelope.Payload, err = sealVaultPayload(vaultKey, envelope, payload)
	if err != nil {
		return nil, nil, err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, nil, err
	}
	return encoded, vaultKey, nil
}

func (s *VaultStore) Unlock(encoded []byte, password string) (OpenedVault, []byte, error) {
	envelope, err := parseVaultEnvelope(encoded)
	if err != nil {
		return OpenedVault{}, nil, err
	}
	salt, err := decodeVaultField(envelope.KDF.Salt)
	if err != nil {
		return OpenedVault{}, nil, ErrVaultCorrupted
	}
	wrappingKey := argon2.IDKey([]byte(password), salt, envelope.KDF.Time, envelope.KDF.MemoryKiB, envelope.KDF.Parallelism, chacha20poly1305.KeySize)
	vaultKey, err := openVaultBytes(wrappingKey, envelope.WrappedKey, vaultKeyAssociatedData(envelope))
	if err != nil {
		return OpenedVault{}, nil, ErrInvalidVaultPassword
	}
	payload, err := openVaultPayload(vaultKey, envelope)
	if err != nil {
		return OpenedVault{}, nil, err
	}
	return OpenedVault{VaultID: envelope.VaultID, Revision: envelope.Revision, Payload: payload}, vaultKey, nil
}

func (s *VaultStore) Open(encoded, vaultKey []byte) (OpenedVault, error) {
	envelope, err := parseVaultEnvelope(encoded)
	if err != nil {
		return OpenedVault{}, err
	}
	payload, err := openVaultPayload(vaultKey, envelope)
	if err != nil {
		return OpenedVault{}, err
	}
	return OpenedVault{VaultID: envelope.VaultID, Revision: envelope.Revision, Payload: payload}, nil
}

// Seal retains the Vault ID and wrapping envelope while replacing only the
// authenticated workspace payload with a fresh nonce and opaque revision.
func (s *VaultStore) Seal(encoded, vaultKey []byte, payload VaultPayload) ([]byte, error) {
	envelope, err := parseVaultEnvelope(encoded)
	if err != nil {
		return nil, err
	}
	if _, err := openVaultPayload(vaultKey, envelope); err != nil {
		return nil, err
	}
	envelope.Revision = uuid.NewString()
	envelope.Payload, err = sealVaultPayload(vaultKey, envelope, payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

// ChangePassword rewraps only the random Vault Key. The workspace ciphertext
// is not decrypted or rewritten.
func (s *VaultStore) ChangePassword(encoded, vaultKey []byte, newPassword string) ([]byte, error) {
	if newPassword == "" {
		return nil, errors.New("new encryption password is required")
	}
	envelope, err := parseVaultEnvelope(encoded)
	if err != nil {
		return nil, err
	}
	if _, err := openVaultPayload(vaultKey, envelope); err != nil {
		return nil, err
	}
	salt, err := randomBytes(16)
	if err != nil {
		return nil, err
	}
	envelope.KDF = vaultKDFParameters{
		Algorithm: vaultKDFAlgorithm, Version: 1,
		Salt:      base64.RawStdEncoding.EncodeToString(salt),
		MemoryKiB: s.memoryKiB, Time: s.time, Parallelism: s.parallelism,
	}
	wrappingKey := argon2.IDKey([]byte(newPassword), salt, envelope.KDF.Time, envelope.KDF.MemoryKiB, envelope.KDF.Parallelism, chacha20poly1305.KeySize)
	envelope.WrappedKey, err = sealVaultBytes(wrappingKey, vaultKey, vaultKeyAssociatedData(envelope))
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

func VaultID(encoded []byte) (string, error) {
	envelope, err := parseVaultEnvelope(encoded)
	if err != nil {
		return "", err
	}
	return envelope.VaultID, nil
}

func parseVaultEnvelope(encoded []byte) (vaultEnvelope, error) {
	var envelope vaultEnvelope
	if len(encoded) == 0 || json.Unmarshal(encoded, &envelope) != nil {
		return vaultEnvelope{}, ErrVaultCorrupted
	}
	if envelope.FormatVersion != vaultFormatVersion ||
		envelope.KDF.Version != 1 ||
		envelope.KDF.Algorithm != vaultKDFAlgorithm ||
		envelope.WrappedKey.Algorithm != vaultCipherAlgorithm ||
		envelope.Payload.Algorithm != vaultCipherAlgorithm {
		return vaultEnvelope{}, ErrVaultVersionUnsupported
	}
	if envelope.VaultID == "" || envelope.Revision == "" ||
		envelope.KDF.MemoryKiB < vaultMinimumMemoryKiB ||
		envelope.KDF.Time < vaultMinimumTime ||
		envelope.KDF.Parallelism == 0 {
		return vaultEnvelope{}, ErrVaultCorrupted
	}
	return envelope, nil
}

func sealVaultPayload(key []byte, envelope vaultEnvelope, payload VaultPayload) (vaultCiphertext, error) {
	payload.SchemaVersion = vaultSchemaVersion
	plain, err := json.Marshal(payload)
	if err != nil {
		return vaultCiphertext{}, err
	}
	return sealVaultBytes(key, plain, payloadAssociatedData(envelope))
}

func openVaultPayload(key []byte, envelope vaultEnvelope) (VaultPayload, error) {
	plain, err := openVaultBytes(key, envelope.Payload, payloadAssociatedData(envelope))
	if err != nil {
		return VaultPayload{}, ErrVaultCorrupted
	}
	var payload VaultPayload
	if json.Unmarshal(plain, &payload) != nil {
		return VaultPayload{}, ErrVaultCorrupted
	}
	if payload.SchemaVersion != vaultSchemaVersion {
		return VaultPayload{}, ErrVaultVersionUnsupported
	}
	if payload.Passwords == nil {
		payload.Passwords = map[string]string{}
	}
	return payload, nil
}

func sealVaultBytes(key, plain, associatedData []byte) (vaultCiphertext, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return vaultCiphertext{}, err
	}
	nonce, err := randomBytes(aead.NonceSize())
	if err != nil {
		return vaultCiphertext{}, err
	}
	sealed := aead.Seal(nil, nonce, plain, associatedData)
	return vaultCiphertext{
		Algorithm:  vaultCipherAlgorithm,
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(sealed),
	}, nil
}

func openVaultBytes(key []byte, value vaultCiphertext, associatedData []byte) ([]byte, error) {
	if value.Algorithm != vaultCipherAlgorithm {
		return nil, ErrVaultVersionUnsupported
	}
	nonce, err := decodeVaultField(value.Nonce)
	if err != nil {
		return nil, err
	}
	ciphertext, err := decodeVaultField(value.Ciphertext)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil || len(nonce) != chacha20poly1305.NonceSizeX {
		return nil, ErrVaultCorrupted
	}
	return aead.Open(nil, nonce, ciphertext, associatedData)
}

func decodeVaultField(value string) ([]byte, error) {
	decoded, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		return nil, ErrVaultCorrupted
	}
	return decoded, nil
}

func randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return nil, fmt.Errorf("generate secure random bytes: %w", err)
	}
	return value, nil
}

func vaultKeyAssociatedData(envelope vaultEnvelope) []byte {
	return []byte(fmt.Sprintf("ssh-connect:vault-key:%s:%d", envelope.VaultID, envelope.FormatVersion))
}

func payloadAssociatedData(envelope vaultEnvelope) []byte {
	return []byte(fmt.Sprintf("ssh-connect:workspace:%s:%s:%d:%d", envelope.VaultID, envelope.Revision, envelope.FormatVersion, vaultSchemaVersion))
}
