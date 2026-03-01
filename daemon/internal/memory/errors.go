package memory

import "errors"

var (
	ErrRootDirRequired   = errors.New("memory root directory is not configured")
	ErrManifestInvalid   = errors.New("invalid memory.yaml manifest")
	ErrAttachmentMissing = errors.New("memory attachment not found")
	ErrViewNotPrepared   = errors.New("memory view not prepared")
	ErrExportTooLarge    = errors.New("memory export exceeds size limit")
	ErrDiskSpaceLow      = errors.New("insufficient disk space for export")
	ErrExportBusy        = errors.New("memory export concurrency limit reached")
	ErrDistillBusy       = errors.New("memory distill already running for scope")
	ErrDistillDirtyTree  = errors.New("memory distill requires clean git working tree")
)
