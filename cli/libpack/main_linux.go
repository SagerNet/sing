package main

import (
	"archive/tar"
	_ "embed"
	"encoding/hex"
	"io"
	"io/fs"
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

func main() {
	err := run0()
	if err != nil {
		logrus.Fatal(err)
	}
}

func run0() error {
	os.Setenv("LD_LIBRARY_PATH", os.ExpandEnv("$LD_LIBRARY_PATH:/usr/local/lib:$PWD"))

	if len(os.Args) == 1 {
		logrus.Fatal("missing executable path")
	}

	realPath, err := filepath.Abs(os.Args[1])
	if err != nil {
		return E.Cause(err, os.Args[1], " not found")
	}

	if len(os.Args) == 2 {
		os.Args = append(os.Args, realPath)
	}

	realName := filepath.Base(realPath)

	output := os.Args[2]
	output, err = filepath.Abs(output)
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
		/*if strings.HasPrefix(it.FullName, "/usr/lib") {
			logrus.Info("skipped ", it.FullName)
			return false
		}*/
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
		var linkName string
		if lib.FileInfo.Mode()&fs.ModeSymlink != 0 {
			linkName, err = os.Readlink(lib.FullName)
			if err != nil {
				return err
			}
			linkName = filepath.Base(libName)
			if libName == linkName {
				continue
			}
			logrus.Info(">> ", libName, " => ", linkName)
		} else {
			logrus.Info(">> ", libName)
		}
		header, err := tar.FileInfoHeader(lib.FileInfo, linkName)
		if err != nil {
			return err
		}
		header.Name = libName
		err = tarWriter.WriteHeader(header)
		if err != nil {
			return err
		}
		libFile, err := os.Open(lib.FullName)
		if err != nil {
			return err
		}
		_, err = io.CopyN(tarWriter, libFile, header.Size)
		libFile.Close()
		if err != nil {
			return err
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
	execName = "`+realName+`"
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
	err = runAs(cachePath, "go", "build", "-o", output, "-trimpath", "-ldflags", "-s -w -buildid=", ".")
	if err != nil {
		return err
	}
	return nil
}

func runAs(dir string, name string, args ...string) error {
	var argc []string
	for _, arg := range args {
		if strings.Contains(arg, " ") {
			argc = append(argc, "\"", arg, "\"")
		} else {
			argc = append(argc, arg)
		}
	}
	logrus.Info(">> ", name, " ", strings.Join(argc, " "))

	command := exec.Command(name, args...)
	command.Dir = dir
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = os.Environ()
	command.Env = append(command.Env, "CGO_ENABLED=0")

	return command.Run()
}
