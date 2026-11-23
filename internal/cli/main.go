package cli

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"
	"time"

	"github.com/qingyu-org/Gist/internal/actions"
	"github.com/qingyu-org/Gist/internal/auth/webauthn"
	"github.com/qingyu-org/Gist/internal/config"
	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/git"
	"github.com/qingyu-org/Gist/internal/index"
	"github.com/qingyu-org/Gist/internal/moderation"
	"github.com/qingyu-org/Gist/internal/settings"
	"github.com/qingyu-org/Gist/internal/ssh"
	"github.com/qingyu-org/Gist/internal/web/handlers/metrics"
	"github.com/qingyu-org/Gist/internal/web/server"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

var CmdVersion = cli.Command{
	Name:  "version",
	Usage: "Print the version of QingyuGist",
	Action: func(c *cli.Context) error {
		fmt.Println("QingyuGist (清羽Gist) " + config.QingyuGistVersion)
		return nil
	},
}

var CmdStart = cli.Command{
	Name:  "start",
	Usage: "Start QingyuGist server",
	Action: func(ctx *cli.Context) error {
		stopCtx, stop := signal.NotifyContext(ctx.Context, syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		Initialize(ctx)

		httpServer := server.NewServer(config.Env("QYG_DEV") == "1")
		go httpServer.Start()
		go ssh.Start()
		stopCron := actions.StartCron()

		var metricsServer *metrics.Server
		if config.C.MetricsEnabled {
			metricsServer = metrics.NewServer()
			go metricsServer.Start()
		}

		<-stopCtx.Done()
		stopCron()
		shutdown(httpServer, metricsServer)
		return nil
	},
}

// ConfigFlag is accepted and ignored. Git hooks installed by earlier releases
// invoke the binary with `--config=<home>/symlinks/config.yml`; keeping the flag
// lets repositories created before the switch to environment-only configuration
// keep pushing. The home directory is resolved from QYG_HOME_INTERNAL, which the
// server exports for every child process.
var ConfigFlag = cli.StringFlag{
	Name:    "config",
	Aliases: []string{"c"},
	Usage:   "Deprecated and ignored; configuration comes from QYG_* environment variables",
}

func App() error {
	app := cli.NewApp()
	app.Name = "QingyuGist"
	app.Usage = "A self-hosted pastebin powered by Git."
	app.HelpName = "qingyugist"

	app.Commands = []*cli.Command{&CmdVersion, &CmdStart, &CmdHook, &CmdAdmin, &CmdKeys, &CmdShell}
	app.DefaultCommand = CmdStart.Name
	app.Flags = []cli.Flag{
		&ConfigFlag,
	}
	return app.Run(os.Args)
}

func Initialize(ctx *cli.Context) {
	fmt.Println("QingyuGist (清羽Gist) " + config.QingyuGistVersion)

	if err := config.InitConfig(os.Stdout); err != nil {
		panic(err)
	}
	if err := os.MkdirAll(filepath.Join(config.GetHomeDir()), 0755); err != nil {
		panic(err)
	}

	config.SetupSecretKey()

	config.InitLog()

	gitVersion, err := git.GetGitVersion()
	if err != nil {
		log.Fatal().Err(err).Send()
	}

	if ok, err := config.CheckGitVersion(gitVersion); err != nil {
		log.Warn().Err(err).Msg("Could not determine the git version; some features may not work as expected")
	} else if !ok {
		log.Warn().Msg("Git version may be too old, as QingyuGist has not been tested prior git version 2.28 and some features would not work. " +
			"Current git version: " + gitVersion)
	}

	homePath := config.GetHomeDir()
	log.Info().Msg("Data directory: " + homePath)

	if err := git.InitGitConfig(); err != nil {
		log.Warn().Err(err).Msgf("Failed to change the host's git global config, ensure to add to `safe.directory` the path %s, and `receive.advertisePushOptions` is set to true.", homePath)
	}

	if err := createSymlink(homePath); err != nil {
		log.Fatal().Err(err).Msg("Failed to create symlinks")
	}

	if err := os.MkdirAll(filepath.Join(homePath, "sessions"), 0755); err != nil {
		log.Fatal().Err(err).Send()
	}
	if err := os.MkdirAll(filepath.Join(homePath, "repos"), 0755); err != nil {
		log.Fatal().Err(err).Send()
	}
	if err := os.MkdirAll(filepath.Join(homePath, "tmp", "repos"), 0755); err != nil {
		log.Fatal().Err(err).Send()
	}
	if err := os.MkdirAll(filepath.Join(homePath, "custom"), 0755); err != nil {
		log.Fatal().Err(err).Send()
	}

	db.DeprecationDBFilename()
	if err := db.Setup(config.C.DBUri); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}

	if err := webauthn.Init(config.C.ExternalUrl); err != nil {
		log.Error().Err(err).Msg("Failed to initialize WebAuthn")
	}

	// Runtime settings live in the database and must be loaded before anything
	// that renders a template or reads a feature flag.
	if err := settings.Init(); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize instance settings")
	}

	moderation.Start()

	index.DepreactionIndexDirname()
	if index.IndexEnabled() {
		// A mapping change recreates the index files, so refill them from the
		// database as soon as the new indexer is live.
		index.SetRebuildHook(func() { actions.RunOnce(actions.IndexGists) })
		if settings.LastIndexRun().IsZero() {
			settings.SetLastIndexRun(time.Now())
		}
		go index.NewIndexer(index.IndexType())
	}
}

func shutdown(httpServer *server.Server, metricsServer *metrics.Server) {
	log.Info().Msg("Shutting down database...")
	if err := db.Close(); err != nil {
		log.Error().Err(err).Msg("Failed to close database")
	}

	if index.IndexEnabled() {
		log.Info().Msg("Shutting down index...")
		index.Close()
	}

	httpServer.Stop()

	if metricsServer != nil {
		metricsServer.Stop()
	}

	log.Info().Msg("Shutdown complete")
}

func createSymlink(homePath string) error {
	if err := os.MkdirAll(filepath.Join(homePath, "symlinks"), 0755); err != nil {
		return err
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	symlinkExePath := path.Join(config.GetHomeDir(), "symlinks", "qingyugist")
	if _, err := os.Lstat(symlinkExePath); err == nil {
		if err := os.Remove(symlinkExePath); err != nil {
			return err
		}
	}
	if err = os.Symlink(exePath, symlinkExePath); err != nil {
		// Windows only lets privileged processes create symlinks. A copy works
		// just as well for the git hooks that shell out to `qingyugist`, so fall
		// back to it instead of refusing to start.
		if copyErr := copyExecutable(exePath, symlinkExePath); copyErr != nil {
			return err
		}
	}

	// Git hooks written by the pre-fork binary point at the old name. Keep that
	// path as an alias so existing repositories keep working after an upgrade.
	if config.LegacyExecutableName != "qingyugist" {
		legacyExePath := path.Join(config.GetHomeDir(), "symlinks", config.LegacyExecutableName)
		if _, err := os.Lstat(legacyExePath); err == nil {
			_ = os.Remove(legacyExePath)
		}
		if err := os.Symlink("qingyugist", legacyExePath); err != nil {
			_ = os.Link(symlinkExePath, legacyExePath)
		}
	}

	// Releases before the environment-only config kept a config.yml symlink here
	// for the git hooks. Remove a leftover copy so it cannot mislead anyone.
	symlinkConfigPath := path.Join(config.GetHomeDir(), "symlinks", "config.yml")
	if _, err := os.Lstat(symlinkConfigPath); err == nil {
		_ = os.Remove(symlinkConfigPath)
	}

	return nil
}

// copyExecutable copies src to dst with the permissions needed to run it. It is
// the fallback for platforms (notably unprivileged Windows) where symlinks are
// unavailable.
func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err = out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// Windows will not rename over an existing file, so drop the target first.
	_ = os.Remove(dst)
	return os.Rename(tmp, dst)
}
