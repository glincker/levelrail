package stacksync

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/iac"
)

// File is one document read from the checkout; Name is relative to the sync path.
type File struct {
	Name    string
	Content string
}

var docExt = map[string]bool{".yaml": true, ".yml": true, ".json": true}

// ReadTree reads the documents under rel without following any symlink. A
// symlink anywhere on the path or under it refuses the whole sync.
func ReadTree(root, rel string, lim Limits) ([]File, error) {
	clean, err := iac.CleanSyncPath(rel)
	if err != nil {
		return nil, &CodedError{Code: iac.CodeSyncPathInvalid, Msg: "path leaves the checkout"}
	}
	if err := checkRepoSize(root, lim.MaxRepoBytes); err != nil {
		return nil, err
	}
	base := root
	if clean != "" {
		for _, seg := range strings.Split(clean, "/") {
			base = filepath.Join(base, seg)
			fi, err := os.Lstat(base)
			if err != nil || fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir() {
				return nil, &CodedError{Code: iac.CodeSyncPathInvalid, Msg: "path is not a directory in the checkout"}
			}
		}
	}
	var files []File
	walkErr := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name, _ := filepath.Rel(base, p)
		name = filepath.ToSlash(name)
		if d.Type()&fs.ModeSymlink != 0 {
			return &CodedError{Code: iac.CodeSyncPathInvalid, Msg: "symlink at " + name}
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !docExt[strings.ToLower(path.Ext(name))] {
			return nil
		}
		if len(files) >= lim.MaxFiles {
			return &CodedError{Code: iac.CodeSyncLimitsExceeded, Msg: "too many documents under the sync path"}
		}
		data, err := readCapped(p, lim.MaxFileBytes)
		if err != nil {
			return err
		}
		files = append(files, File{Name: name, Content: string(data)})
		return nil
	})
	if walkErr != nil {
		var ce *CodedError
		if errors.As(walkErr, &ce) {
			return nil, ce
		}
		return nil, &CodedError{Code: iac.CodeSyncPathInvalid, Msg: "sync path could not be read"}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

func readCapped(p string, limit int64) ([]byte, error) {
	f, err := os.Open(p) //nolint:gosec // p comes from WalkDir under the checkout and is never a symlink
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, &CodedError{Code: iac.CodeSyncLimitsExceeded, Msg: "a document is larger than the per file limit"}
	}
	return data, nil
}

// checkRepoSize sums regular files without following links.
func checkRepoSize(root string, limit int64) error {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if total += fi.Size(); total > limit {
			return &CodedError{Code: iac.CodeSyncLimitsExceeded, Msg: "repository is larger than the size limit"}
		}
		return nil
	})
	var ce *CodedError
	if errors.As(err, &ce) {
		return ce
	}
	if err != nil {
		return &CodedError{Code: iac.CodeSyncPathInvalid, Msg: "checkout could not be read"}
	}
	return nil
}
