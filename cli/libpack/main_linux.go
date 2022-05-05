package main

import (
	"archive/tar"
	_ "embed"
	"encoding/hex"
	"github.com/sagernet/sing"
	"github.com/sagernet/sing/common/log"
	"github.com/spf13/cobra"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sirupsen/logrus"
	"github.com/u-root/u-root/pkg/ldd"
)

var logger = log.NewLogger("libpack")

var packageName string
var executablePath string
var outputPath string

func main() {
	command := &cobra.Command{
		Use:     "libpack",
		Version: sing.VersionStr,
		Run:     run0,
	}
	command.Flags().StringVarP(&executablePath, "input", "i", "", "input path (required)")
	command.MarkFlagRequired("file")
	command.Flags().StringVarP(&outputPath, "output", "o", "", "output path (default: input path)")
	command.Flags().StringVarP(&packageName, "package", "p", "", "package name (default: executable name)")

	if err := command.Execute(); err != nil {
		logger.Fatal(err)
	}
}

func run0(cmd *cobra.Command, args []string) {
	err := run1()
	if err != nil {
		logger.Fatal(err)
	}
}

var skipPaths = []string{
	"/usr/lib",
	"/usr/lib64",
}

func run1() error {
	os.Setenv("LD_LIBRARY_PATH", os.ExpandEnv("$LD_LIBRARY_PATH:/usr/local/lib:$PWD"))

	realPath, err := filepath.Abs(executablePath)
	if err != nil {
		return E.Cause(err, executablePath, " not found")
	}

	realName := filepath.Base(realPath)

	if outputPath == "" {
		outputPath = realPath
	}

	if packageName == "" {
		packageName = realName
	}

	outputPath, err = filepath.Abs(outputPath)
	if err != nil {
		return err
	}

	cachePath, err := os.MkdirTemp("", "libpack")
	if err != nil {
		return err
	}
	defer os.RemoveAll(cachePath)
	contentFile, err := os.Create(cachePath + "/content")
	if err != nil {
		return err
	}

	writer, err := zstd.NewWriter(contentFile, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return err
	}

	var ldName string
	tarWriter := tar.NewWriter(writer)
	libs, err := ldd.Ldd([]string{realPath})
	if err != nil {
		return err
	}
	libs = common.Filter(libs, func(it *ldd.FileInfo) bool {
		if strings.HasPrefix(it.Name(), "ld-") {
			ldName = it.Name()
			return false
		}
		for _, path := range skipPaths {
			if strings.HasPrefix(it.FullName, path) {
				logrus.Info(">> skipped ", it.FullName)
				return false
			}
		}
		return true
	})
	if ldName == "" {
		for _, lib := range libs {
			logrus.Info(lib.FullName)
		}
		logrus.Fatal("not a dynamically linked executable i thk")
	}
	sort.Slice(libs, func(i, j int) bool {
		lName := filepath.Base(libs[i].FullName)
		rName := filepath.Base(libs[j].FullName)
		if lName == realName {
			return false
		} else if rName == realName {
			return true
		}
		return lName > rName
	})
	for _, lib := range libs {
		libName := filepath.Base(lib.FullName)
		header, err := tar.FileInfoHeader(lib.FileInfo, "")
		if err != nil {
			return err
		}
		libFile, err := os.Open(lib.FullName)
		if err != nil {
			return err
		}
		if strings.HasPrefix(libName, "libc") {
			logger.Info(">> ", libName)

			header.Name = libName
			err = tarWriter.WriteHeader(header)
			if err != nil {
				return err
			}
			_, err = io.CopyN(tarWriter, libFile, header.Size)
			if err != nil {
				return err
			}
			libFile.Close()
			continue
		} else {
			cacheFile, err := os.Create(cachePath + "/" + libName)
			if err != nil {
				return err
			}
			_, err = io.CopyN(cacheFile, libFile, header.Size)
			if err != nil {
				return err
			}
			libFile.Close()
			cacheFile.Close()
			err = runAs(cachePath, "strip", "-s", libName)
			if err != nil {
				return err
			}
			cacheFile, err = os.Open(cachePath + "/" + libName)
			if err != nil {
				return err
			}
			libInfo, err := cacheFile.Stat()
			if err != nil {
				return err
			}

			if libName == realName {
				libName = packageName
			}

			logger.Info(">> ", libName)

			header.Name = libName
			header.Size = libInfo.Size()
			err = tarWriter.WriteHeader(header)
			if err != nil {
				return err
			}
			_, err = io.CopyN(tarWriter, cacheFile, header.Size)
			if err != nil {
				return err
			}
			cacheFile.Close()
		}
	}
	err = tarWriter.Close()
	if err != nil {
		return err
	}
	err = writer.Close()
	if err != nil {
		return err
	}
	err = contentFile.Close()
	if err != nil {
		return err
	}

	hash, err := common.SHA224File(cachePath + "/content")
	if err != nil {
		return err
	}

	err = common.WriteFile(cachePath+"/main.go", []byte(`package main

import (
	"archive/tar"
	"bytes"
	_ "embed"
	"io"
	"log"
	"os"
	"syscall"

	"github.com/klauspost/compress/zstd"
	"github.com/sagernet/sing/common"
)

//go:embed content
var content []byte

const (
	execName = "`+packageName+`"
	hash     = "`+hex.EncodeToString(hash)+`"
)

var (
	basePath = os.TempDir() + "/.sing/" + execName
	dirPath  = basePath + "/" + hash
	execPath = dirPath + "/" + execName
)

func main() {
	log.SetFlags(0)

	err := os.Setenv("LD_LIBRARY_PATH", dirPath)
	if err != nil {
		log.Fatalln(err)
	}

	err = main0()
	if err != nil {
		log.Fatalln(err)
	}
}

//noinspection GoBoolExpressions
func main0() error {
	if !common.FileExists(dirPath + "/" + hash) {
		os.RemoveAll(basePath)
	}
	if common.FileExists(execPath) {
		return syscall.Exec(execPath, os.Args, os.Environ())
	}
	os.RemoveAll(basePath)
	os.MkdirAll(dirPath, 0o755)
	reader, err := zstd.NewReader(bytes.NewReader(content))
	if err != nil {
		return err
	}
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if header.FileInfo().IsDir() {
			os.MkdirAll(dirPath+"/"+header.Name, 0o755)
			continue
		}
		libFile, err := os.OpenFile(dirPath+"/"+header.Name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		_, err = io.CopyN(libFile, tarReader, header.Size)
		if err != nil {
			return err
		}
		libFile.Close()
	}
	reader.Close()
	return syscall.Exec(execPath, os.Args, os.Environ())
}`))
	if err != nil {
		return err
	}
	err = common.WriteFile(cachePath+"/go.mod", []byte(`module output
	
	go 1.18
	
	require (
		github.com/klauspost/compress latest
		github.com/sagernet/sing latest
	)`))
	if err != nil {
		return err
	}
	err = runAs(cachePath, "go", "mod", "tidy")
	if err != nil {
		return err
	}
	logrus.Info(">> ", outputPath)
	err = runAs(cachePath, "go", "build", "-o", outputPath, "-trimpath", "-ldflags", "-s -w -buildid=", ".")
	if err != nil {
		return err
	}
	return nil
}

func runAs(dir string, name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Dir = dir
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = os.Environ()
	command.Env = append(command.Env, "CGO_ENABLED=0")

	return command.Run()
}
