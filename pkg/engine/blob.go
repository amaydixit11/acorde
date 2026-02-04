package engine

import (
	"github.com/amaydixit11/acorde/internal/blob"
)

// CID is a Content Identifier (hash of blob content)
type CID = blob.CID

// BlobStore provides content-addressed storage for large files
type BlobStore interface {
	// StoreBlob stores a blob and returns its content ID
	StoreBlob(data []byte) (CID, error)
	
	// GetBlob retrieves a blob by its content ID
	GetBlob(cid CID) ([]byte, error)
	
	// HasBlob checks if a blob exists
	HasBlob(cid CID) bool
	
	// DeleteBlob removes a blob
	DeleteBlob(cid CID) error
}

// blobWrapper wraps the internal blob store
type blobWrapper struct {
	store *blob.Store
}

// NewBlobStore creates a blob store at the given data directory
func NewBlobStore(dataDir string) (BlobStore, error) {
	store, err := blob.NewStore(dataDir)
	if err != nil {
		return nil, err
	}
	return &blobWrapper{store: store}, nil
}

func (b *blobWrapper) StoreBlob(data []byte) (CID, error) {
	return b.store.PutWithSubdir(data)
}

func (b *blobWrapper) GetBlob(cid CID) ([]byte, error) {
	return b.store.Get(cid)
}

func (b *blobWrapper) HasBlob(cid CID) bool {
	return b.store.Has(cid)
}

func (b *blobWrapper) DeleteBlob(cid CID) error {
	return b.store.Delete(cid)
}
