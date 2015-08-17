package subako

import (
	"os"
	"io"
	"io/ioutil"
	"path"
	"path/filepath"
	"log"
	"errors"
	"fmt"
	"time"
	"encoding/json"

	"github.com/fsouza/go-dockerclient"
)


const endpoint = "unix:///var/run/docker.sock"


type BuilderConfig struct {
	virtualUsrDir		string
	tmpBaseDir			string
	packagesDir			string
	packagePrefix		string
	installBasePrefix	string
}


type BuilderContext struct {
	client				*docker.Client

	virtualUsrDir		string
	tmpBaseDir			string
	packagesDir			string
	packagePrefix		string
	installBasePrefix	string
}

func MakeBuilderContext(config *BuilderConfig) (*BuilderContext, error) {
	client, err := docker.NewClient(endpoint)
	if err != nil {
		return nil, err
	}

	if !Exists(config.virtualUsrDir) {
		if err := os.Mkdir(config.virtualUsrDir, 0755); err != nil {
			return nil, err
		}
	}

	if !Exists(config.tmpBaseDir) {
		if err := os.Mkdir(config.tmpBaseDir, 0755); err != nil {
			return nil, err
		}
	}

	if !Exists(config.packagesDir) {
		if err := os.Mkdir(config.packagesDir, 0755); err != nil {
			return nil, err
		}
	}

	return &BuilderContext{
		client: client,
		virtualUsrDir: config.virtualUsrDir,
		tmpBaseDir: config.tmpBaseDir,
		packagesDir: config.packagesDir,
		packagePrefix: config.packagePrefix,
		installBasePrefix: config.installBasePrefix,
	}, nil
}


type BuildResult struct {
	PkgFileName		string	`json:"pkg_file_name"`
	PkgName			string	`json:"pkg_name"`
	PkgVersion		string	`json:"pkg_version"`
	DisplayVersion	string	`json:"display_version"`

	hostInstallBase		string
	hostInstallPrefix	string
	duration			time.Duration
}

type IntermediateContainerInfo struct {
	ContainerID			string
	KillContainerFunc	func() error
}

func (ctx *BuilderContext) build(
	procConfig			IPackageBuildConfig,
	procConfigSetsDir	string,
	writePipe			io.Writer,
	intermediateCh		chan<-IntermediateContainerInfo,
) (*BuildResult, error) {
	const inContainerPkgConfigsDir = "/etc/pkgconfigs"
	const inContainerCurPkgConfigsDir = "/etc/current_pkgconfig"

	const inContainerWorkDir = "/root"
	const inContainerBuiltPkgsDir = "/etc/torigoya_pkgs"

	//
	workDirGen := path.Join(ctx.tmpBaseDir, procConfig.makeWorkDirName())
	workDir, err := exactFilePath(workDirGen)
	if err != nil {
		return nil, err
	}

	inContainerInstalledPath :=
		path.Join(ctx.installBasePrefix, procConfig.makePackagePathName())
	inContainerInstallScriptPath :=
		path.Join(inContainerCurPkgConfigsDir, "install.sh")

	containerOpt := docker.CreateContainerOptions{
		Config: &docker.Config{
			Image: "torigoya_builder/base",
			//AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			//Tty:          true,
			WorkingDir: inContainerWorkDir,
			Env: []string{
				"PATH=/bin:/usr/bin:/usr/local/bin/",
				"TR_REUSE_FLAG=0",
				"TR_VERSION=" + string(procConfig.GetVersion()),
				"TR_INSTALL_PREFIX=" + inContainerInstalledPath,
				"TR_PACKAGE_NAME=" + string(procConfig.GetGenPkgName()),
				"TR_TARGET_SYSTEM=" + procConfig.GetTargetSystem(),
				"TR_TARGET_ARCH=" + procConfig.GetTargetArch(),
				"TR_INSTALL_PATH=" + ctx.installBasePrefix,
				"TR_PKGS_PATH=" + inContainerBuiltPkgsDir,
				"TR_CPU_CORE=2",	// TODO: fix
				"TR_PACKAGE_PREFIX=" + ctx.packagePrefix,
			},
			Cmd: []string{"bash", inContainerInstallScriptPath},
		},
	}
	log.Printf("Build (%s, %s) <- %v", procConfig.GetName(), procConfig.GetDepVersion(), procConfig.GetDepPackage())
	if procConfig.GetDepPackage() != nil {
		ap := procConfig.GetDepPackage()
		containerOpt.Config.Env = append(containerOpt.Config.Env, []string{
			"TR_DEP_PKG_NAME=" + string(ap.Name),
			"TR_DEP_PKG_VERSION=" + string(ap.Version),
			"TR_DEP_PKG_GEN_NAME=" + ap.GeneratedPackageName,
			"TR_DEP_PKG_GEN_VERSION=" + ap.GeneratedPackageVersion,
			"TR_DEP_PKG_DISP_VERSION=" + ap.DisplayVersion,
			"TR_DEP_PKG_PATH=" + ap.InstallPrefix,
		}...)
	}

	container, err := ctx.client.CreateContainer(containerOpt)
	if err != nil {
		log.Printf("Error: CreateContainer: %v\n", err)
		return nil, err
	}
	defer ctx.client.RemoveContainer(docker.RemoveContainerOptions{
		ID: container.ID,
		Force: true,
	})
	intermediateCh <- IntermediateContainerInfo{
		ContainerID: container.ID,
		KillContainerFunc: func() error {
			log.Printf("Kill Container %s", container.ID)
			return ctx.client.KillContainer(docker.KillContainerOptions{
				ID: container.ID,
			})
		},
	}

	log.Printf("Attach Container => %s\n", container.ID)
	attachOpt := docker.AttachToContainerOptions{
		Container: container.ID,
		OutputStream: writePipe,
		ErrorStream: writePipe,
		Logs: true,
		Stream: true,
		Stdout: true,
		Stderr: true,
	}
	go func(ctx *BuilderContext, opt docker.AttachToContainerOptions) {
		if err := ctx.client.AttachToContainer(opt); err != nil {
			log.Printf("Error: AttachToContainer: %v\n", err)
		}
	}(ctx, attachOpt)

	log.Printf("Start Container\n")
	startT := time.Now()
	hostConfig := &docker.HostConfig{
		Binds: []string {
			procConfigSetsDir + ":" + inContainerPkgConfigsDir + ":ro",				// readonly
			procConfig.GetBasePath() + ":" + inContainerCurPkgConfigsDir + ":ro",	// readonly
			workDir + ":" + inContainerWorkDir,
			ctx.virtualUsrDir + ":" + ctx.installBasePrefix,						// user can use compilers from ctx.installBasePrefix
			ctx.packagesDir + ":" + inContainerBuiltPkgsDir,
		},
	}
	if err := ctx.client.StartContainer(container.ID, hostConfig); err != nil {
		log.Printf("Error: CreateContainer: %v\n", err)
		return nil, err
	}

	status_code, err := ctx.client.WaitContainer(container.ID)
	log.Printf("status_code = %d / %v\n", status_code, err)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(writePipe, "Exit Status => %d\n", status_code)
	endT := time.Now()
	if status_code != 0 {
		return nil, errors.New("Container status code is not 0")
	}

	//
	resultJsonName := fmt.Sprintf("result-%s-%s.json", procConfig.GetGenPkgName(), procConfig.GetVersion())
	file, err := ioutil.ReadFile(filepath.Join(ctx.packagesDir, resultJsonName))
    if err != nil {
        log.Printf("JSON read error: %v\n", err)
        return nil, fmt.Errorf("failed to read result %s", resultJsonName)
    }

	var br BuildResult
	if err := json.Unmarshal(file, &br); err != nil {
		log.Printf("File error: %v\n", err)
        return nil, err
	}
	br.hostInstallBase = ctx.installBasePrefix			//
	br.hostInstallPrefix = inContainerInstalledPath		//
	br.duration = endT.Sub(startT)

	log.Println("BUILD RESULT", br)

	return &br, nil
}
