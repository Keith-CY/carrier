package integration

import (
	"path/filepath"

	"carrier/shared/work"
)

type Paths struct {
	Root   string
	DBPath string
}

func ResolvePaths() (Paths, error) {
	roots, err := work.ResolveRoots()
	if err != nil {
		return Paths{}, err
	}
	root := filepath.Join(roots.App, "integrations")
	return Paths{
		Root:   root,
		DBPath: filepath.Join(root, "state.sqlite"),
	}, nil
}
