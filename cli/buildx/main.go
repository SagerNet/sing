package main

import (
	"archive/tar"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/klauspost/compress/zip"
	"github.com/sagernet/sing/common/log"
	"github.com/spf13/cobra"
	"github.com/ulikunitz/xz"
)

var logger = log.NewLogger("buildx")

var (
	appName      string
	appPath      string
	outputDir    string
	buildRelease bool
)

func main() {
	command := &cobra.Command{
		Use: "buildx",
		Run: build,
	}
	command.Flags().StringVar(&appName, "name", "", "binary name")
	command.Flags().StringVar(&appPath, "path", "", "program path")
	command.Flags().StringVar(&outputDir, "output", "bin", "output directory")
	command.Flags().BoolVar(&buildRelease, "release", false, "build release archives")

	if err := command.Execute(); err != nil {
		logger.Fatal(err)
	}
}

func build(cmd *cobra.Command, args []string) {
	err := buildOne(appName, appPath, outputDir, buildRelease)
	if err != nil {
		logger.Fatal(err)
	}
}

type goBuildTarget struct {
	name   string
	os     string
	arch   string
	extEnv []string
}

func t(name string, os string, arch string, extEnv ...string) goBuildTarget {
	return goBuildTarget{
		name:   name,
		os:     os,
		arch:   arch,
		extEnv: extEnv,
	}
}

var commonTargets = []goBuildTarget{
	t("android-x86_64", "android", "amd64", "GOAMD64=v3"),
	t("android-x86", "android", "386"),
	t("android-arm64-v8a", "android", "arm64"),
	t("android-armeabi-v7a", "android", "arm", "GOARM=7"),

	t("linux-x86_64", "linux", "amd64", "GOAMD64=v3"),
	t("linux-x86", "linux", "386"),
	t("linux-arm64-v8a", "linux", "arm64"),
	t("linux-armeabi-v7a", "linux", "arm", "GOARM=7"),

	t("windows-x86_64", "windows", "amd64", "GOAMD64=v3"),
	t("windows-x86", "windows", "386"),
	t("windows-arm64-v8a", "windows", "arm64"),

	t("darwin-x86_64", "darwin", "amd64", "GOAMD64=v3"),
	t("darwin-arm64-v8a", "darwin", "arm64"),
}

func buildOne(app string, appPath string, outputDir string, release bool) error {
	tmpDir, err := os.MkdirTemp("", "sing-build")
	if err != nil {
		return err
	}
	for _, t := range commonTargets {
		logger.Info(">> ", app, "-", t.name)

		env := t.extEnv
		env = append(env, "GOOS+"+t.os)
		env = append(env, "GOARCH="+t.arch)
		env = append(env, "CGO_ENABLED=0")

		if !release {
			err = run("go", env, "build",
				"-trimpath", "-ldflags", "-w -s -buildid=",
				"-v", "-o", outputDir+"/"+app+"-"+t.name, appPath)
			if err != nil {
				return err
			}
			continue
		}

		cache := tmpDir + "/" + app + "-" + t.name
		if t.os == "windows" {
			cache += ".exe"
		}
		output := outputDir + "/" + app + "-" + t.name
		err = run("go", env, "build",
			"-trimpath", "-ldflags", "-w -s -buildid=",
			"-v", "-o", cache, appPath)
		if err != nil {
			return err
		}
		binary, err := os.Open(cache)
		if err != nil {
			return err
		}
		binaryInfo, err := os.Stat(cache)
		if err != nil {
			return err
		}
		err = os.MkdirAll("bin", 0o755)
		if err != nil {
			return err
		}
		if t.os != "windows" {
			binaryPackage, err := os.Create(output + ".tar.xz")
			if err != nil {
				return err
			}
			xzConfig := &xz.WriterConfig{
				DictCap:  1 << 26,
				CheckSum: xz.SHA256,
			}
			xzWrtier, err := xzConfig.NewWriter(binaryPackage)
			if err != nil {
				return err
			}
			tarWriter := tar.NewWriter(xzWrtier)
			tarHeader, err := tar.FileInfoHeader(binaryInfo, "")
			if err != nil {
				return err
			}
			tarHeader.Name = filepath.Base(tarHeader.Name)
			err = tarWriter.WriteHeader(tarHeader)
			if err != nil {
				return err
			}
			_, err = io.Copy(tarWriter, binary)
			if err != nil {
				return err
			}
			tarWriter.Close()
			xzWrtier.Close()
			binaryPackage.Close()
			binary.Close()
		} else {
			binaryPackage, err := os.Create(output + ".zip")
			if err != nil {
				return err
			}
			zipHeader, err := zip.FileInfoHeader(binaryInfo)
			if err != nil {
				return err
			}
			zipHeader.Name = filepath.Base(zipHeader.Name)
			zipWriter := zip.NewWriter(binaryPackage)
			fileWriter, err := zipWriter.CreateHeader(zipHeader)
			if err != nil {
				return err
			}
			_, err = io.Copy(fileWriter, binary)
			if err != nil {
				return err
			}
			zipWriter.Close()
			binaryPackage.Close()
			binary.Close()
		}
	}
	os.RemoveAll(tmpDir)
	return nil
}

func run(name string, env []string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = append(env, os.Environ()...)
	return command.Run()
}
