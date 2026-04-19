package hfclient

import "time"

// TreeEntry is one entry returned by the HF tree API.
type TreeEntry struct {
	Path    string // POSIX, validated
	Size    int64
	SHA1    string // git blob OID; verifier for non-LFS files
	SHA256  string // lfs.oid; verifier for LFS files; "" for non-LFS
	XetHash string // optional, present if Xet-migrated; informational only
	IsLFS   bool
}

// FileEntry is the manifest-side expectation for one file.
type FileEntry struct {
	Path   string
	Size   int64
	SHA256 string
	SHA1   string
	IsLFS  bool
}

// SnapshotEvent is one event emitted during a snapshot download.
type SnapshotEvent struct {
	Kind           string // "tree-fetched"|"file-start"|"file-bytes"|"file-done"|"snapshot-done"
	File           string
	BytesDone      int64
	BytesTotal     int64
	FilesDone      int
	FilesTotal     int
	AggregateDone  int64
	AggregateTotal int64
}

// SnapshotProgress is the callback shape consumers register.
type SnapshotProgress func(SnapshotEvent)

// SnapshotResult is returned on successful DownloadSnapshot.
type SnapshotResult struct {
	Dir         string
	CommitSHA   string
	FilesByPath map[string]int64
	Duration    time.Duration
}
