package node

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/libp2p/go-libp2p/core/crypto"
)

func GenerateSwarmKey(w io.Writer) error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	fmt.Fprintln(w, "/key/swarm/psk/1.0.0/")
	fmt.Fprintln(w, "/base16/")
	fmt.Fprintln(w, hex.EncodeToString(key))
	return nil
}

func LoadOrCreateIdentity(path string) (crypto.PrivKey, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return crypto.UnmarshalPrivateKey(data)
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	raw, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(path, raw, 0600); err != nil {
		return nil, fmt.Errorf("save identity key: %w", err)
	}

	return priv, nil
}
