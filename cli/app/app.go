package app

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/docker/libcompose/project"
	"github.com/docker/libcompose/project/options"
)

// ProjectAction is an adapter to allow the use of ordinary functions as libcompose actions.
// Any function that has the appropriate signature can be register as an action on a codegansta/cli command.
//
// cli.Command{
//		Name:   "ps",
//		Usage:  "List containers",
//		Action: app.WithProject(factory, app.ProjectPs),
//	}
type ProjectAction func(project project.APIProject, c *cli.Context)

// BeforeApp is an action that is executed before any cli command.
func BeforeApp(c *cli.Context) error {
	if c.GlobalBool("verbose") {
		logrus.SetLevel(logrus.DebugLevel)
	}
	logrus.Warning("Note: This is an experimental alternate implementation of the Compose CLI (https://github.com/docker/compose)")
	return nil
}

// WithProject is a helper function to create a cli.Command action with a ProjectFactory.
func WithProject(factory ProjectFactory, action ProjectAction) func(context *cli.Context) {
	return func(context *cli.Context) {
		p, err := factory.Create(context)
		if err != nil {
			logrus.Fatalf("Failed to read project: %v", err)
		}
		action(p, context)
	}
}

// ProjectPs lists the containers.
func ProjectPs(p project.APIProject, c *cli.Context) {
	qFlag := c.Bool("q")
	allInfo, err := p.Ps(qFlag, c.Args()...)
	if err != nil {
		logrus.Fatal(err)
	}
	os.Stdout.WriteString(allInfo.String(!qFlag))
}

// ProjectPort prints the public port for a port binding.
func ProjectPort(p project.APIProject, c *cli.Context) {
	if len(c.Args()) != 2 {
		logrus.Fatalf("Please pass arguments in the form: SERVICE PORT")
	}

	index := c.Int("index")
	protocol := c.String("protocol")
	serviceName := c.Args()[0]
	privatePort := c.Args()[1]

	port, err := p.Port(index, protocol, serviceName, privatePort)
	if err != nil {
		logrus.Fatal(err)
	}
	fmt.Println(port)
}

// ProjectStop stops all services.
func ProjectStop(p project.APIProject, c *cli.Context) {
	err := p.Stop(c.Int("timeout"), c.Args()...)
	if err != nil {
		logrus.Fatal(err)
	}
}

// ProjectDown brings all services down (stops and clean containers).
func ProjectDown(p project.APIProject, c *cli.Context) {
	options := options.Down{
		RemoveVolume: c.Bool("v"),
	}
	err := p.Down(options, c.Args()...)
	if err != nil {
		logrus.Fatal(err)
	}
}

// ProjectBuild builds or rebuilds services.
func ProjectBuild(p project.APIProject, c *cli.Context) {
	config := options.Build{
		NoCache: c.Bool("no-cache"),
	}
	err := p.Build(config, c.Args()...)
	if err != nil {
		logrus.Fatal(err)
	}
}

// ProjectCreate creates all services but do not start them.
func ProjectCreate(p project.APIProject, c *cli.Context) {
	options := options.Create{
		NoRecreate:    c.Bool("no-recreate"),
		ForceRecreate: c.Bool("force-recreate"),
		NoBuild:       c.Bool("no-build"),
	}
	err := p.Create(options, c.Args()...)
	if err != nil {
		logrus.Fatal(err)
	}
}

// ProjectUp brings all services up.
func ProjectUp(p project.APIProject, c *cli.Context) {
	options := options.Up{
		Create: options.Create{
			NoRecreate:    c.Bool("no-recreate"),
			ForceRecreate: c.Bool("force-recreate"),
			NoBuild:       c.Bool("no-build"),
		},
	}
	err := p.Up(options, c.Args()...)
	if err != nil {
		logrus.Fatal(err)
	}
	if !c.Bool("d") {
		signalChan := make(chan os.Signal, 1)
		cleanupDone := make(chan bool)
		signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
		errChan := make(chan error)
		go func() {
			errChan <- p.Log(true, c.Args()...)
		}()
		go func() {
			select {
			case <-signalChan:
				fmt.Printf("\nGracefully stopping...\n")
				ProjectStop(p, c)
				cleanupDone <- true
			case err := <-errChan:
				if err != nil {
					logrus.Fatal(err)
				}
				cleanupDone <- true
			}
		}()
		<-cleanupDone
	}
}

// ProjectRun runs a given command within a service's container.
func ProjectRun(p project.APIProject, c *cli.Context) {
	if len(c.Args()) == 1 {
		logrus.Fatal("No service specified")
	}

	serviceName := c.Args()[0]
	commandParts := c.Args()[1:]

	exitCode, err := p.Run(serviceName, commandParts)
	if err != nil {
		logrus.Fatal(err)
	}

	os.Exit(exitCode)
}

// ProjectStart starts services.
func ProjectStart(p project.APIProject, c *cli.Context) {
	err := p.Start(c.Args()...)
	if err != nil {
		logrus.Fatal(err)
	}
}

// ProjectRestart restarts services.
func ProjectRestart(p project.APIProject, c *cli.Context) {
	err := p.Restart(c.Int("timeout"), c.Args()...)
	if err != nil {
		logrus.Fatal(err)
	}
}

// ProjectLog gets services logs.
func ProjectLog(p project.APIProject, c *cli.Context) {
	err := p.Log(c.Bool("follow"), c.Args()...)
	if err != nil {
		logrus.Fatal(err)
	}
}

// ProjectPull pulls images for services.
func ProjectPull(p project.APIProject, c *cli.Context) {
	err := p.Pull(c.Args()...)
	if err != nil {
		logrus.Fatal(err)
	}
}

// ProjectDelete deletes services.
func ProjectDelete(p project.APIProject, c *cli.Context) {
	stoppedContainers, err := p.ListStoppedContainers(c.Args()...)
	if err != nil {
		logrus.Fatal(err)
	}
	if len(stoppedContainers) == 0 {
		fmt.Println("No stopped containers")
		return
	}
	if !c.Bool("force") {
		fmt.Printf("Going to remove %v\nAre you sure? [yN]\n", strings.Join(stoppedContainers, ", "))
		var answer string
		_, err := fmt.Scanln(&answer)
		if err != nil {
			logrus.Fatal(err)
		}
		if answer != "y" && answer != "Y" {
			return
		}
	}
	options := options.Delete{
		RemoveVolume: c.Bool("v"),
	}
	err = p.Delete(options, c.Args()...)
	if err != nil {
		logrus.Fatal(err)
	}
}

// ProjectKill forces stop service containers.
func ProjectKill(p project.APIProject, c *cli.Context) {
	err := p.Kill(c.String("signal"), c.Args()...)
	if err != nil {
		logrus.Fatal(err)
	}
}

// ProjectPause pauses service containers.
func ProjectPause(p project.APIProject, c *cli.Context) {
	err := p.Pause(c.Args()...)
	if err != nil {
		logrus.Fatal(err)
	}
}

// ProjectUnpause unpauses service containers.
func ProjectUnpause(p project.APIProject, c *cli.Context) {
	err := p.Unpause(c.Args()...)
	if err != nil {
		logrus.Fatal(err)
	}
}

// ProjectScale scales services.
func ProjectScale(p project.APIProject, c *cli.Context) {
	servicesScale := map[string]int{}
	for _, arg := range c.Args() {
		kv := strings.SplitN(arg, "=", 2)
		if len(kv) != 2 {
			logrus.Fatalf("Invalid scale parameter: %s", arg)
		}

		name := kv[0]

		count, err := strconv.Atoi(kv[1])
		if err != nil {
			logrus.Fatalf("Invalid scale parameter: %v", err)
		}

		servicesScale[name] = count
	}

	err := p.Scale(c.Int("timeout"), servicesScale)
	if err != nil {
		logrus.Fatal(err)
	}
}
