package sync

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/skip2/go-qrcode"
)

// InvitePrefix is the URL scheme for vaultd invites
const InvitePrefix = "vaultd://"

// DefaultInviteExpiry is how long invites are valid
const DefaultInviteExpiry = 24 * time.Hour

// PeerInvite contains data needed to connect to a peer
type PeerInvite struct {
	PeerID    string   `json:"p"`    // Peer ID
	Addresses []string `json:"a"`    // Multiaddrs
	PublicKey []byte   `json:"k"`    // Public key for verification
	CreatedAt int64    `json:"c"`    // Unix timestamp
	ExpiresAt int64    `json:"e"`    // Expiry timestamp
	Signature []byte   `json:"s"`    // Signature over above fields
	Key       []byte   `json:"y,omitempty"` // Encryption key (optional)
}

// CreateInvite generates a signed invite for this host
func CreateInvite(h host.Host, expiry time.Duration) (*PeerInvite, error) {
	now := time.Now()

	// Get addresses (limit to 2 most useful ones for QR size)
	addrs := h.Addrs()
	addrStrs := make([]string, 0, 2)
	for _, a := range addrs {
		str := a.String()
		// Prefer non-loopback addresses
		if !strings.Contains(str, "127.0.0.1") && !strings.Contains(str, "::1") {
			addrStrs = append(addrStrs, str)
			if len(addrStrs) >= 2 {
				break
			}
		}
	}
	// Fallback to any address if none found
	if len(addrStrs) == 0 && len(addrs) > 0 {
		addrStrs = append(addrStrs, addrs[0].String())
	}

	// Get public key
	pubKey := h.Peerstore().PubKey(h.ID())
	if pubKey == nil {
		return nil, fmt.Errorf("no public key found")
	}
	pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	invite := &PeerInvite{
		PeerID:    h.ID().String(),
		Addresses: addrStrs,
		PublicKey: pubKeyBytes,
		CreatedAt: now.Unix(),
		ExpiresAt: now.Add(expiry).Unix(),
	}

	// Sign the invite
	privKey := h.Peerstore().PrivKey(h.ID())
	if privKey == nil {
		return nil, fmt.Errorf("no private key found")
	}

	dataToSign := invite.signableData()
	sig, err := privKey.Sign(dataToSign)
	if err != nil {
		return nil, fmt.Errorf("failed to sign invite: %w", err)
	}
	invite.Signature = sig

	return invite, nil
}

// signableData returns the data that gets signed
func (i *PeerInvite) signableData() []byte {
	data := fmt.Sprintf("%s|%s|%d|%d",
		i.PeerID,
		strings.Join(i.Addresses, ","),
		i.CreatedAt,
		i.ExpiresAt,
	)
	return []byte(data)
}

// Encode serializes the invite to a compact string
func (i *PeerInvite) Encode() (string, error) {
	data, err := json.Marshal(i)
	if err != nil {
		return "", err
	}
	return InvitePrefix + base64.RawURLEncoding.EncodeToString(data), nil
}

// ToQR generates a QR code PNG for the invite
func (i *PeerInvite) ToQR() ([]byte, error) {
	// Use minimal format for QR: just peer ID and first address
	minimalCode := i.ToMinimalCode()
	return qrcode.Encode(minimalCode, qrcode.Low, 256)
}

// ToQRString generates an ASCII art QR code for terminal display
func (i *PeerInvite) ToQRString() (string, error) {
	minimalCode := i.ToMinimalCode()
	qr, err := qrcode.New(minimalCode, qrcode.Low)
	if err != nil {
		return "", err
	}
	return qr.ToSmallString(false), nil
}

// ToMinimalCode returns a short code for QR: vaultd://PEERID@ADDR
func (i *PeerInvite) ToMinimalCode() string {
	addr := ""
	if len(i.Addresses) > 0 {
		addr = i.Addresses[0]
	}
	return fmt.Sprintf("%s%s@%s", InvitePrefix, i.PeerID, addr)
}

// ParseInvite decodes and validates an invite string
func ParseInvite(s string) (*PeerInvite, error) {
	// Remove prefix
	if !strings.HasPrefix(s, InvitePrefix) {
		return nil, fmt.Errorf("invalid invite format: missing prefix")
	}
	data := strings.TrimPrefix(s, InvitePrefix)

	// Decode base64
	jsonData, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("invalid invite encoding: %w", err)
	}

	// Parse JSON
	var invite PeerInvite
	if err := json.Unmarshal(jsonData, &invite); err != nil {
		return nil, fmt.Errorf("invalid invite data: %w", err)
	}

	// Check expiry
	if time.Now().Unix() > invite.ExpiresAt {
		return nil, fmt.Errorf("invite expired")
	}

	// Verify signature
	pubKey, err := crypto.UnmarshalPublicKey(invite.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

	valid, err := pubKey.Verify(invite.signableData(), invite.Signature)
	if err != nil || !valid {
		return nil, fmt.Errorf("invalid signature")
	}

	// Verify peer ID matches public key
	derivedID, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to derive peer ID: %w", err)
	}
	if derivedID.String() != invite.PeerID {
		return nil, fmt.Errorf("peer ID mismatch")
	}

	return &invite, nil
}

// ToPeerAddrInfo converts the invite to libp2p peer address info
func (i *PeerInvite) ToPeerAddrInfo() (*peer.AddrInfo, error) {
	peerID, err := peer.Decode(i.PeerID)
	if err != nil {
		return nil, fmt.Errorf("invalid peer ID: %w", err)
	}

	addrInfo := &peer.AddrInfo{ID: peerID}
	for _, addrStr := range i.Addresses {
		// We'll parse these when connecting
		_ = addrStr
	}

	return addrInfo, nil
}

// IsExpired returns true if the invite has expired
func (i *PeerInvite) IsExpired() bool {
	return time.Now().Unix() > i.ExpiresAt
}

// ExpiresIn returns the duration until the invite expires
func (i *PeerInvite) ExpiresIn() time.Duration {
	return time.Until(time.Unix(i.ExpiresAt, 0))
}

// unused but required for ed25519 import
var _ = ed25519.PublicKey{}
