package container

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/registry"
	"github.com/otiai10/copy"
	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/bundler/archive"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/fileutil"
)

//go:embed Dockerfile.tmpl
var dockerfileTemplate string

type dockerfileConfig struct {
	Base string
}

// BuildImageFromBundle creates an image based on an existing bundle.
func BuildImageFromBundle(bundlePath string) (string, error) {
	buildContextDir, err := prepareBuildContext(bundlePath)
	if err != nil {
		return "", err
	}
	return buildImageFromDir(buildContextDir)
}

// UploadImage uploads an image to a registry.
func UploadImage(imageID string, regConf *api.RegistryConfig, imageName string) error {
	log.Debugf("Start uploading image %s to %s", imageID, regConf.URL)

	dockerClient, err := GetDockerClient()
	if err != nil {
		return err
	}
	// TODO: make the building/pushing cancellable with SIGnals
	ctx := context.Background()

	remoteTag := fmt.Sprintf("%s:%s", strings.ToLower(imageName), imageID)
	log.Debugf("Tag used for upload: %s", remoteTag)

	err = dockerClient.ImageTag(ctx, imageID, remoteTag)
	if err != nil {
		return errors.WithStack(err)
	}

	regAuth, err := registry.EncodeAuthConfig(*regConf.Auth)
	if err != nil {
		return errors.WithStack(err)
	}

	opts := types.ImagePushOptions{RegistryAuth: regAuth}
	res, err := dockerClient.ImagePush(ctx, remoteTag, opts)
	if err != nil {
		return errors.WithStack(err)
	}
	defer res.Close()

	_, err = parseImageBuildOutput(res)
	if err != nil {
		return err
	}

	return nil
}

// prepareBuildContext takes a existing artifact bundle, extracts it
// and adds needed files/information.
func prepareBuildContext(bundlePath string) (string, error) {
	// extract bundle to a temporary directory
	buildContextDir, err := os.MkdirTemp("", "bundle-extract")
	if err != nil {
		return "", errors.WithStack(err)
	}

	err = archive.Extract(bundlePath, buildContextDir)
	if err != nil {
		return "", errors.WithMessagef(err, "Failed to extract bundle to %s", buildContextDir)
	}

	// read metadata from bundle to use information for building
	// the right image
	metadata, err := archive.MetadataFromPath(filepath.Join(buildContextDir, archive.MetadataFileName))
	if err != nil {
		return "", errors.WithMessage(err, "Failed to read bundle.yml")
	}

	// add additional files needed for the image
	// eg. build instructions and cifuzz executables
	err = createDockerfile(buildContextDir, metadata.Docker)
	if err != nil {
		return "", err
	}
	err = copyCifuzz(buildContextDir)
	if err != nil {
		return "", errors.WithMessagef(err, "Failed to copy CI Fuzz binaries to %s", buildContextDir)
	}

	log.Debugf("Prepared build context for fuzz container image at %s", buildContextDir)

	return buildContextDir, nil
}

// builds an image based on an existing directory
func buildImageFromDir(buildContextDir string) (string, error) {
	imageTar, err := CreateImageTar(buildContextDir)
	if err != nil {
		return "", err
	}
	defer fileutil.Cleanup(imageTar.Name())

	dockerClient, err := GetDockerClient()
	if err != nil {
		return "", err
	}

	ctx := context.Background()
	opts := types.ImageBuildOptions{
		Dockerfile:  "Dockerfile",
		Platform:    "linux/amd64",
		Remove:      true,
		ForceRemove: true,
		Tags:        []string{"cifuzz"},
	}
	res, err := dockerClient.ImageBuild(ctx, imageTar, opts)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer res.Body.Close()

	imageID, err := parseImageBuildOutput(res.Body)
	if err != nil {
		return "", err
	}
	log.Debugf("Created fuzz container image with ID %s and tags %s", imageID, opts.Tags)
	return imageID, nil
}

// creates a tar archive that can be used for building an image
// based on a given directory
func CreateImageTar(buildContextDir string) (*os.File, error) {
	imageTar, err := os.CreateTemp("", "*_image.tar")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer imageTar.Close()

	writer := archive.NewTarArchiveWriter(imageTar, false)
	defer writer.Close()
	err = writer.WriteDir("", buildContextDir)
	if err != nil {
		return nil, err
	}

	// the client.BuildImage from docker expects an unclosed io.Reader / os.File
	file, err := os.Open(imageTar.Name())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return file, nil
}

func createDockerfile(buildContextDir string, baseImage string) error {
	// copy cifuzz linux executable to buildContextDir
	cifuzzPath, err := runfiles.Finder.CIFuzzLinuxExecutablePath()
	if err != nil {
		return err
	}
	err = copy.Copy(cifuzzPath, filepath.Join(buildContextDir, "cifuzz_linux"))
	if err != nil {
		return errors.WithStack(err)
	}

	// open Dockerfile
	dockerFilePath := filepath.Join(buildContextDir, "Dockerfile")
	dockerfile, err := os.OpenFile(dockerFilePath, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return errors.WithStack(err)
	}
	defer dockerfile.Close()

	// write Dockerfile using base image
	dockerConfig := dockerfileConfig{
		Base: baseImage,
	}
	tmpl, err := template.New("Dockerfile").Parse(dockerfileTemplate)
	if err != nil {
		return errors.WithStack(err)
	}
	err = tmpl.Execute(dockerfile, dockerConfig)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func copyCifuzz(buildContextDir string) error {
	// Add the CIFuzz binaries to the bundle if the version is "dev".
	// TODO: this should work even if internal doesn't exist
	exists, err := fileutil.Exists("../../build/bin")
	if exists && err == nil {

		dest := filepath.Join(buildContextDir, "internal", "cifuzz_binaries")
		src := "../../build/bin"
		err = copy.Copy(src, dest)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}
