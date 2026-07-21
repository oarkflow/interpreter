package secretr

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oarkflow/secretr/pkg/core/compliance"
	"github.com/oarkflow/secretr/pkg/core/identity"
	"github.com/oarkflow/secretr/pkg/core/secrets"
	"github.com/oarkflow/secretr/pkg/storage"
	"github.com/oarkflow/secretr/pkg/types"
)

// dlpRuleName identifies the single default DLP rule this plugin creates to
// flag hardcoded-secret-shaped strings in SPL source. It only matches
// clearly secret-shaped patterns (cloud keys, tokens, private key headers) -
// not broad PII patterns like SSNs/credit cards, which are a different
// concern from "don't hardcode secrets in scripts".
const dlpRuleName = "spl-hardcoded-secrets"

// hardcodedSecretPatterns are builtin pattern names (see
// github.com/oarkflow/secretr/pkg/core/compliance's DLPEngine) selected for a
// low false-positive rate. "generic_api_key" (any 32-64 char alnum run) is
// deliberately excluded from the default rule - it would flag ordinary
// hashes/UUIDs-without-dashes far too often - but is still available to
// secretr_scan() for callers who explicitly ask for it.
var hardcodedSecretPatterns = []string{
	"aws_access_key",
	"aws_secret_key",
	"github_token",
	"private_key",
	"jwt_token",
}

type vaultHandle struct {
	store    *storage.Store
	vault    *secrets.Vault
	dlp      *compliance.DLPEngine
	actorID  types.ID
	dataDir  string
}

var (
	handleOnce sync.Once
	handle     *vaultHandle
	handleErr  error
)

func dataDir() string {
	if dir := strings.TrimSpace(os.Getenv("SECRETR_DATA_DIR")); dir != "" {
		return dir
	}
	return ".secretr-data"
}

// getVault lazily bootstraps (or loads, on later calls / later process runs
// against the same SECRETR_DATA_DIR) the local storage, a single bootstrap
// identity, the secrets vault, and the DLP engine used for
// BlockHardcodedSecrets detection. Bootstrapping happens once per process;
// errors are cached so repeated calls after a failure don't retry storage
// I/O on every builtin invocation.
func getVault() (*vaultHandle, error) {
	handleOnce.Do(func() {
		handle, handleErr = bootstrap()
	})
	return handle, handleErr
}

func bootstrap() (*vaultHandle, error) {
	dir := dataDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secretr: create data dir: %w", err)
	}

	store, err := storage.NewStore(storage.Config{Path: dir})
	if err != nil {
		return nil, fmt.Errorf("secretr: open store: %w", err)
	}

	ctx := context.Background()
	identityManager := identity.NewManager(identity.ManagerConfig{
		Store:          store,
		SessionTimeout: 24 * time.Hour,
	})

	actorID, err := bootstrapIdentity(ctx, dir, identityManager)
	if err != nil {
		store.Close()
		return nil, err
	}

	keyProvider, err := newFileKeyProvider(dir)
	if err != nil {
		store.Close()
		return nil, err
	}

	vault := secrets.NewVault(secrets.VaultConfig{Store: store, KeyManager: keyProvider})

	dlp := compliance.NewDLPEngine(compliance.DLPEngineConfig{Store: store})
	if err := ensureDefaultDLPRule(ctx, dlp, actorID); err != nil {
		store.Close()
		return nil, err
	}

	return &vaultHandle{store: store, vault: vault, dlp: dlp, actorID: actorID, dataDir: dir}, nil
}

// bootstrapIdentity loads the single local identity used to own every
// secret/rule this plugin creates, persisting its ID on first run so later
// process runs (against the same data dir) reuse the same actor instead of
// creating a fresh one every time.
func bootstrapIdentity(ctx context.Context, dir string, mgr *identity.Manager) (types.ID, error) {
	idPath := filepath.Join(dir, "identity.id")
	if data, err := os.ReadFile(idPath); err == nil {
		id := types.ID(strings.TrimSpace(string(data)))
		if id != "" {
			if _, err := mgr.GetIdentity(ctx, id); err == nil {
				return id, nil
			}
			// Fall through and create a fresh one if the persisted ID no
			// longer resolves (e.g. the store was reset independently).
		}
	}

	password, err := randomHex(24)
	if err != nil {
		return "", fmt.Errorf("secretr: generate bootstrap password: %w", err)
	}
	ident, err := mgr.CreateHumanIdentity(ctx, identity.CreateHumanOptions{
		Name:     "spl-interpreter",
		Email:    "spl-interpreter@local",
		Password: password,
		Scopes:   []types.Scope{types.ScopeAdminAll},
	})
	if err != nil {
		return "", fmt.Errorf("secretr: bootstrap identity: %w", err)
	}
	if err := os.WriteFile(idPath, []byte(ident.ID), 0o600); err != nil {
		return "", fmt.Errorf("secretr: persist bootstrap identity: %w", err)
	}
	return ident.ID, nil
}

func ensureDefaultDLPRule(ctx context.Context, dlp *compliance.DLPEngine, actorID types.ID) error {
	rules, err := dlp.ListRules(ctx)
	if err != nil {
		return fmt.Errorf("secretr: list DLP rules: %w", err)
	}
	for _, r := range rules {
		if r.Name == dlpRuleName {
			return nil
		}
	}
	_, err = dlp.CreateRule(ctx, compliance.CreateDLPRuleOptions{
		Name:           dlpRuleName,
		Description:    "Flags hardcoded-secret-shaped strings in SPL source (see BlockHardcodedSecrets).",
		PatternType:    compliance.PatternTypeBuiltIn,
		Patterns:       hardcodedSecretPatterns,
		Classification: compliance.ClassificationSecret,
		Severity:       compliance.DLPSeverityCritical,
		Actions:        []compliance.DLPAction{compliance.DLPActionBlock},
		AppliesTo:      []string{"all"},
		CreatedBy:      actorID,
	})
	if err != nil {
		return fmt.Errorf("secretr: create default DLP rule: %w", err)
	}
	return nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	const hex = "0123456789abcdef"
	out := make([]byte, n*2)
	for i, b := range buf {
		out[i*2] = hex[b>>4]
		out[i*2+1] = hex[b&0x0f]
	}
	return string(out), nil
}

// fileKeyProvider implements secrets.KeyProvider with a single symmetric key
// persisted to <dataDir>/vault.key (0600), generated once on first use. This
// is a local/dev-mode key management scheme, not a production KMS - secretr
// itself documents stronger KeyProvider implementations for production use;
// see this plugin's README.
type fileKeyProvider struct {
	id  types.ID
	key []byte
}

const fileKeyID = types.ID("local-file-key-1")

func newFileKeyProvider(dir string) (*fileKeyProvider, error) {
	path := filepath.Join(dir, "vault.key")
	if data, err := os.ReadFile(path); err == nil && len(data) == 32 {
		return &fileKeyProvider{id: fileKeyID, key: data}, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("secretr: generate vault key: %w", err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("secretr: persist vault key: %w", err)
	}
	return &fileKeyProvider{id: fileKeyID, key: key}, nil
}

// GetKey returns a fresh copy of the key on every call. Callers (e.g.
// secrets.Vault.Create/Get) call security.Zeroize on the returned slice
// once they're done with it, which overwrites it in place - returning the
// same backing array here would silently destroy this provider's
// persisted key after its very first use.
func (k *fileKeyProvider) GetKey(_ context.Context, _ types.ID) ([]byte, error) {
	return append([]byte(nil), k.key...), nil
}

func (k *fileKeyProvider) GetCurrentKeyID(_ context.Context) (types.ID, error) {
	return k.id, nil
}
