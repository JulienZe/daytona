//go:build !windows

// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"fmt"
	"os"
	"path"

	"github.com/daytonaio/daytona/pkg/agent"
	"github.com/daytonaio/daytona/pkg/agent/config"
	"github.com/daytonaio/daytona/pkg/agent/git"
	"github.com/daytonaio/daytona/pkg/agent/ssh"
	"github.com/daytonaio/daytona/pkg/agent/tailscale"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var hostModeFlag bool

var AgentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Start the agent process",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		agentMode := config.ModeProject

		if hostModeFlag {
			agentMode = config.ModeHost
		}

		config, err := config.GetConfig(agentMode)
		if err != nil {
			log.Fatal(err)
		}

		git := &git.Service{
			ProjectDir:        config.ProjectDir,
			GitConfigFileName: path.Join(os.Getenv("HOME"), ".gitconfig"),
		}

		sshServer := &ssh.Server{
			ProjectDir:        config.ProjectDir,
			DefaultProjectDir: os.Getenv("HOME"),
		}

		tailscaleHostname := fmt.Sprintf("%s-%s", config.WorkspaceId, config.ProjectName)
		if hostModeFlag {
			tailscaleHostname = config.WorkspaceId
		}

		tailscaleServer := &tailscale.Server{
			Hostname:  tailscaleHostname,
			ServerUrl: config.Server.Url,
		}

		agent := agent.Agent{
			Config:    config,
			Git:       git,
			Ssh:       sshServer,
			Tailscale: tailscaleServer,
		}

		err = agent.Start()
		if err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	AgentCmd.Flags().BoolVar(&hostModeFlag, "host", false, "Run the agent in host mode")
}
