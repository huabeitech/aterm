// Package keys owns SSH private material and its public catalog. Private bytes
// are persisted atomically by the encrypted vault and never appear in list responses.
package keys

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

const Prefix = "aterm-key://"
const MaxPrivateBytes = 1024 * 1024

// Storage owns durable, encrypted records. Revision checks prevent a mutation
// from overwriting a concurrent workspace restore or vault update.
type Storage interface {
	LoadKeys() ([]BackupKey, string, error)
	SaveKeys(string, []BackupKey) error
}
type Key struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Algorithm           string `json:"algorithm"`
	PublicKey           string `json:"publicKey"`
	Fingerprint         string `json:"fingerprint"`
	PassphraseProtected bool   `json:"passphraseProtected"`
	CreatedAt           string `json:"createdAt"`
}
type CreateInput struct {
	Name       string `json:"name"`
	Algorithm  string `json:"algorithm"`
	Passphrase string `json:"passphrase"`
}
type ImportPreview struct {
	Token              string `json:"token"`
	SourceName         string `json:"sourceName"`
	SuggestedName      string `json:"suggestedName"`
	Algorithm          string `json:"algorithm"`
	Fingerprint        string `json:"fingerprint"`
	RequiresPassphrase bool   `json:"requiresPassphrase"`
}
type pendingImport struct {
	bytes   []byte
	expires time.Time
}
type Store struct {
	mu       sync.Mutex
	closed   bool
	storage  Storage
	keys     []Key
	private  map[string]string
	revision string
	pending  map[string]pendingImport
	now      func() time.Time
}

func NewStore(storage Storage) *Store {
	return &Store{storage: storage, pending: map[string]pendingImport{}, now: time.Now}
}

func (s *Store) unload() {
	s.keys = nil
	clear(s.private)
	s.private = nil
	s.revision = ""
}

func (s *Store) load() error {
	if s.closed {
		return errors.New("managed key storage is closed")
	}
	values, revision, err := s.storage.LoadKeys()
	if err != nil {
		return err
	}
	s.keys = make([]Key, 0, len(values))
	s.private = make(map[string]string, len(values))
	for _, value := range values {
		s.keys = append(s.keys, value.Key)
		s.private[value.ID] = value.PrivateKey
	}
	s.revision = revision
	return nil
}
func validateMetadata(key Key) error {
	if _, err := uuid.Parse(key.ID); err != nil {
		return errors.New("invalid managed SSH key ID")
	}
	if err := validName(key.Name); err != nil {
		return err
	}
	public, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key.PublicKey))
	if err != nil {
		return errors.New("invalid managed SSH public key")
	}
	if algorithmFor(public) != key.Algorithm || ssh.FingerprintSHA256(public) != key.Fingerprint {
		return errors.New("managed SSH key metadata does not match its public key")
	}
	if _, err := time.Parse(time.RFC3339Nano, key.CreatedAt); err != nil {
		return errors.New("invalid managed SSH key creation date")
	}
	return nil
}
func validName(name string) error {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 120 {
		return errors.New("enter a valid name for this SSH key")
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return errors.New("enter a valid name for this SSH key")
		}
	}
	return nil
}
func (s *Store) label(name string) (string, error) {
	if err := s.load(); err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if err := validName(name); err != nil {
		return "", err
	}
	for _, key := range s.keys {
		if strings.EqualFold(key.Name, name) {
			return "", errors.New("a managed SSH key already uses this name")
		}
	}
	return name, nil
}
func algorithmFor(public ssh.PublicKey) string {
	switch public.Type() {
	case ssh.KeyAlgoED25519:
		return "ed25519"
	case ssh.KeyAlgoRSA:
		return "rsa"
	case ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521:
		return "ecdsa"
	}
	return ""
}
func (s *Store) List() []Key {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.unload()
	if err := s.load(); err != nil {
		return []Key{}
	}
	return append([]Key{}, s.keys...)
}
func (s *Store) persist(keys []Key) error {
	values := make([]BackupKey, 0, len(keys))
	for _, key := range keys {
		values = append(values, BackupKey{Key: key, PrivateKey: s.private[key.ID]})
	}
	return s.storage.SaveKeys(s.revision, values)
}
func (s *Store) add(name string, bytes []byte, signer ssh.Signer, protected bool) (Key, error) {
	name, err := s.label(name)
	if err != nil {
		return Key{}, err
	}
	public := signer.PublicKey()
	algorithm := algorithmFor(public)
	if algorithm == "" {
		return Key{}, errors.New("unsupported SSH private key algorithm")
	}
	key := Key{ID: uuid.NewString(), Name: name, Algorithm: algorithm, PublicKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(public))) + " " + name, Fingerprint: ssh.FingerprintSHA256(public), PassphraseProtected: protected, CreatedAt: s.now().UTC().Format(time.RFC3339Nano)}
	s.private[key.ID] = string(bytes)
	next := append(append([]Key{}, s.keys...), key)
	if err := s.persist(next); err != nil {
		return Key{}, err
	}
	s.keys = next
	return key, nil
}
func (s *Store) Create(input CreateInput) (Key, error) {
	if len(input.Passphrase) > 4096 || (input.Passphrase != "" && utf8.RuneCountInString(input.Passphrase) < 12) {
		return Key{}, errors.New("use at least 12 characters for a private key passphrase")
	}
	s.mu.Lock()
	_, err := s.label(input.Name)
	s.unload()
	s.mu.Unlock()
	if err != nil {
		return Key{}, err
	}
	var private crypto.PrivateKey
	switch input.Algorithm {
	case "ed25519":
		_, private, err = ed25519.GenerateKey(rand.Reader)
	case "rsa":
		private, err = rsa.GenerateKey(rand.Reader, 4096)
	case "ecdsa":
		private, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	default:
		return Key{}, errors.New("choose a supported SSH key algorithm")
	}
	if err != nil {
		return Key{}, err
	}
	var block *pem.Block
	if input.Passphrase != "" {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(private, input.Name, []byte(input.Passphrase))
	} else {
		block, err = ssh.MarshalPrivateKey(private, input.Name)
	}
	if err != nil {
		return Key{}, err
	}
	bytes := pem.EncodeToMemory(block)
	defer clear(bytes)
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		return Key{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.unload()
	return s.add(input.Name, bytes, signer, input.Passphrase != "")
}
func (s *Store) cleanPending() {
	for token, pending := range s.pending {
		if !s.now().Before(pending.expires) {
			clear(pending.bytes)
			delete(s.pending, token)
		}
	}
}
func (s *Store) Preview(source string, bytes []byte) (ImportPreview, error) {
	if len(bytes) == 0 || len(bytes) > MaxPrivateBytes {
		return ImportPreview{}, errors.New("choose a private key file smaller than 1 MB")
	}
	signer, err := ssh.ParsePrivateKey(bytes)
	requires := false
	var algorithm, fingerprint string
	if err != nil {
		var missing *ssh.PassphraseMissingError
		if !errors.As(err, &missing) {
			return ImportPreview{}, errors.New("unsupported or invalid private key file")
		}
		requires = true
		if missing.PublicKey != nil {
			algorithm = algorithmFor(missing.PublicKey)
			fingerprint = ssh.FingerprintSHA256(missing.PublicKey)
		}
	} else {
		algorithm = algorithmFor(signer.PublicKey())
		fingerprint = ssh.FingerprintSHA256(signer.PublicKey())
		if algorithm == "" {
			return ImportPreview{}, errors.New("unsupported SSH private key algorithm")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.unload()
	if s.closed {
		return ImportPreview{}, errors.New("managed key storage is closed")
	}
	s.cleanPending()
	if len(s.pending) >= 32 {
		return ImportPreview{}, errors.New("too many pending private key imports")
	}
	token := uuid.NewString()
	s.pending[token] = pendingImport{bytes: append([]byte{}, bytes...), expires: s.now().Add(5 * time.Minute)}
	suggested := source
	for _, suffix := range []string{".pem", ".key", ".ppk"} {
		if strings.HasSuffix(strings.ToLower(suggested), suffix) {
			suggested = suggested[:len(suggested)-len(suffix)]
			break
		}
	}
	return ImportPreview{Token: token, SourceName: source, SuggestedName: suggested, Algorithm: algorithm, Fingerprint: fingerprint, RequiresPassphrase: requires}, nil
}
func (s *Store) Import(token, name, passphrase string) (Key, error) {
	if len(passphrase) > 4096 {
		return Key{}, errors.New("private key passphrase is too long")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.unload()
	s.cleanPending()
	pending, ok := s.pending[token]
	if !ok {
		return Key{}, errors.New("the selected private key expired; choose it again")
	}
	signer, err := ssh.ParsePrivateKey(pending.bytes)
	protected := false
	if err != nil {
		var missing *ssh.PassphraseMissingError
		protected = errors.As(err, &missing)
		if protected {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(pending.bytes, []byte(passphrase))
		}
	}
	if err != nil {
		return Key{}, errors.New("unable to unlock this private key; check its passphrase")
	}
	key, err := s.add(name, pending.bytes, signer, protected)
	if err != nil {
		return Key{}, err
	}
	clear(pending.bytes)
	delete(s.pending, token)
	return key, nil
}
func (s *Store) key(id string) (Key, error) {
	if err := s.load(); err != nil {
		return Key{}, err
	}
	if s.closed {
		return Key{}, errors.New("managed key storage is closed")
	}
	for _, key := range s.keys {
		if key.ID == id {
			return key, nil
		}
	}
	return Key{}, errors.New("managed SSH key not found")
}

// IdentityName resolves only public catalog metadata for credential prompts.
// Missing/deleted identities retain their reference as a diagnostic fallback;
// ReadIdentity is responsible for reporting their actual read failure.
func (s *Store) IdentityName(identity string) string {
	if !strings.HasPrefix(identity, Prefix) {
		return identity
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.unload()
	key, err := s.key(strings.TrimPrefix(identity, Prefix))
	if err != nil {
		return identity
	}
	return key.Name
}

func (s *Store) ReadIdentity(identity string) ([]byte, error) {
	if !strings.HasPrefix(identity, Prefix) {
		return nil, errors.New("identity is not a managed SSH key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.unload()
	id := strings.TrimPrefix(identity, Prefix)
	if _, err := s.key(id); err != nil {
		return nil, err
	}
	value := s.private[id]
	if len(value) == 0 || len(value) > MaxPrivateBytes {
		return nil, errors.New("invalid protected private key material")
	}
	return []byte(value), nil
}
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.unload()
	if _, err := s.key(id); err != nil {
		return err
	}
	next := []Key{}
	for _, key := range s.keys {
		if key.ID != id {
			next = append(next, key)
		}
	}
	if err := s.persist(next); err != nil {
		return err
	}
	s.keys = next
	return nil
}
func (s *Store) Close() {
	s.mu.Lock()
	s.closed = true
	defer s.mu.Unlock()
	defer s.unload()
	for token, pending := range s.pending {
		clear(pending.bytes)
		delete(s.pending, token)
	}
}
