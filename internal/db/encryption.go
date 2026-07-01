package db

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/vinaykumar-v4_PLIBGIT/mssql-mcp-go/internal/infrastructure"

	"github.com/microsoft/go-mssqldb/aecmk/localcert"
)

// configureAlwaysEncrypted wires up Always Encrypted (column encryption) support
// for the connection when it is enabled in the configuration.
//
// go-mssqldb decrypts Always Encrypted columns client-side using a Column Master
// Key (CMK). For local development and on-premises migration work the CMK is
// typically held in a PFX certificate file. This function validates that the
// certificate exists and registers its password with the built-in "pfx" key
// provider (github.com/microsoft/go-mssqldb/aecmk/localcert), which is
// registered globally via its package init.
//
// It returns the connection-string fragment ("columnencryption=true") that must
// be appended to the DSN to actually enable client-side decryption. When
// encryption is disabled it returns an empty string and performs no side effects.
func configureAlwaysEncrypted(cfg infrastructure.EncryptionConfig, logger *slog.Logger) (string, error) {
	if !cfg.Enabled {
		return "", nil
	}

	if cfg.KeyPath == "" {
		return "", fmt.Errorf("always encrypted is enabled but no key path was provided")
	}

	if _, err := os.Stat(cfg.KeyPath); err != nil {
		return "", fmt.Errorf("always encrypted certificate %q is not accessible: %w", cfg.KeyPath, err)
	}

	// Register the PFX password so the provider can open the certificate. The
	// localcert.PfxKeyProvider is registered globally through the package's
	// init function; setting the password with an empty location applies it to
	// every certificate path the provider is asked to open.
	localcert.PfxKeyProvider.SetCertificatePassword(cfg.KeyPath, cfg.KeyPassword)
	localcert.PfxKeyProvider.SetCertificatePassword("", cfg.KeyPassword)

	logger.Info("always encrypted enabled",
		slog.String("key_path", cfg.KeyPath),
		slog.String("provider", "pfx"),
	)

	return "columnencryption=true", nil
}
