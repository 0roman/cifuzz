package artifact

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/pkg/errors"
	"golang.org/x/exp/maps"

	"code-intelligence.com/cifuzz/util/archiveutil"
)

// This struct is used to list all files that should be included in the
// archive.
// - Key:   the desired relative path inside the archive
// - Value: real path to the file in the filesystem.
type FileMap map[string]string

// WriteArchive writes a GZip-compressed TAR to out containing the files and directories given in file map.
// The keys in file map correspond to the path within the archive, the corresponding value is expected to be the
// absolute path of the file or directory on disk.
// Note: WriteArchive *does not* (recursively) traverse directories to add their contents to the archive. If this is
// desired, use AddDirToFileMap to explicitly add the contents to the file map before calling WriteArchive.
func WriteArchive(out io.Writer, fileMap FileMap) error {
	gw := gzip.NewWriter(out)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Sort the archive paths first so that the generated archive is deterministic - map traversals aren't.
	archivePaths := maps.Keys(fileMap)
	sort.Strings(archivePaths)
	for _, archivePath := range archivePaths {
		absPath := fileMap[archivePath]
		err := addToArchive(tw, archivePath, absPath)
		if err != nil {
			return err
		}
	}

	return nil
}

// AddDirToFileMap traverses the directory dir recursively and adds its contents to the file map under the base path
// archiveBasePath.
func AddDirToFileMap(fileMap FileMap, archiveBasePath string, dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return errors.WithStack(err)
		}
		archivePath := filepath.Join(archiveBasePath, relPath)
		// There is no harm in creating tar entries for non-empty directories, even though they are not necessary.
		fileMap[archivePath] = path
		return nil
	})
}

// ExtractArchiveForTestsOnly extracts the bundle to dir.
func ExtractArchiveForTestsOnly(bundle, dir string) error {
	f, err := os.Open(bundle)
	if err != nil {
		return errors.WithStack(err)
	}
	gr, err := gzip.NewReader(f)
	if err != nil {
		return errors.WithStack(err)
	}
	defer gr.Close()
	return archiveutil.Untar(gr, dir)
}

// addToArchive adds the file absPath to the archive under the path archivePath.
func addToArchive(tw *tar.Writer, archivePath, absPath string) error {
	fileOrDir, err := os.Open(absPath)
	if err != nil {
		return errors.Wrapf(err, "failed to add %q at %q", absPath, archivePath)
	}
	defer fileOrDir.Close()
	info, err := fileOrDir.Stat()
	if err != nil {
		return errors.WithStack(err)
	}

	// Since fileOrDir.Stat() follows symlinks, info will not be of type symlink
	// at this point - no need to pass in a non-empty value for link.
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return errors.WithStack(err)
	}
	header.Name = archivePath
	err = tw.WriteHeader(header)
	if err != nil {
		return errors.WithStack(err)
	}

	if !info.Mode().IsRegular() {
		return nil
	}
	_, err = io.Copy(tw, fileOrDir)
	if err != nil {
		return errors.Wrapf(err, "failed to compress file: %s", absPath)
	}

	return nil
}
