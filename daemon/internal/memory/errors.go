package memory

import "errors"

var (
	ErrRootDirRequired   = errors.New("memory root directory is not configured")
	ErrManifestInvalid   = errors.New("invalid memory.yaml manifest")
	ErrAttachmentMissing = errors.New("memory attachment not found")
	ErrViewNotPrepared   = errors.New("memory view not prepared")
)
