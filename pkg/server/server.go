// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/daytonaio/daytona/internal/util"
	"github.com/daytonaio/daytona/pkg/frpc"
	"github.com/daytonaio/daytona/pkg/provider/manager"
	"github.com/daytonaio/daytona/pkg/server/apikeys"
	"github.com/daytonaio/daytona/pkg/server/gitproviders"
	"github.com/daytonaio/daytona/pkg/server/providertargets"
	"github.com/daytonaio/daytona/pkg/server/workspaces"
	"github.com/hashicorp/go-plugin"

	log "github.com/sirupsen/logrus"
)

type ServerInstanceConfig struct {
	Config                Config
	TailscaleServer       TailscaleServer
	ProviderTargetService providertargets.IProviderTargetService
	WorkspaceService      workspaces.IWorkspaceService
	ApiKeyService         apikeys.IApiKeyService
	GitProviderService    gitproviders.IGitProviderService
	ProviderManager       manager.IProviderManager
}

var server *Server

func GetInstance(serverConfig *ServerInstanceConfig) *Server {
	if serverConfig != nil && server != nil {
		log.Fatal("Server already initialized")
	}

	if server == nil {
		if serverConfig == nil {
			log.Fatal("Server not initialized")
		}
		server = &Server{
			config:                serverConfig.Config,
			TailscaleServer:       serverConfig.TailscaleServer,
			ProviderTargetService: serverConfig.ProviderTargetService,
			WorkspaceService:      serverConfig.WorkspaceService,
			ApiKeyService:         serverConfig.ApiKeyService,
			GitProviderService:    serverConfig.GitProviderService,
			ProviderManager:       serverConfig.ProviderManager,
		}
	}

	return server
}

type Server struct {
	config                Config
	TailscaleServer       TailscaleServer
	ProviderTargetService providertargets.IProviderTargetService
	WorkspaceService      workspaces.IWorkspaceService
	ApiKeyService         apikeys.IApiKeyService
	GitProviderService    gitproviders.IGitProviderService
	ProviderManager       manager.IProviderManager
}

func (s *Server) Start(errCh chan error) error {
	err := s.initLogs()
	if err != nil {
		return err
	}

	log.Info("Starting Daytona server")

	// Terminate orphaned provider processes
	err = s.ProviderManager.TerminateProviderProcesses(s.config.ProvidersDir)
	if err != nil {
		log.Errorf("Failed to terminate orphaned provider processes: %s", err)
	}

	err = s.downloadDefaultProviders()
	if err != nil {
		return err
	}

	err = s.registerProviders()
	if err != nil {
		return err
	}

	go func() {
		err := frpc.Connect(frpc.FrpcConnectParams{
			ServerDomain: s.config.Frps.Domain,
			ServerPort:   int(s.config.Frps.Port),
			Name:         fmt.Sprintf("daytona-server-%s", s.config.Id),
			Port:         int(s.config.HeadscalePort),
			SubDomain:    s.config.Id,
		})
		if err != nil {
			errCh <- err
		}
	}()

	go func() {
		err := frpc.Connect(frpc.FrpcConnectParams{
			ServerDomain: s.config.Frps.Domain,
			ServerPort:   int(s.config.Frps.Port),
			Name:         fmt.Sprintf("daytona-server-api-%s", s.config.Id),
			Port:         int(s.config.ApiPort),
			SubDomain:    fmt.Sprintf("api-%s", s.config.Id),
		})
		if err != nil {
			errCh <- err
		}
	}()

	go func() {
		interruptChannel := make(chan os.Signal, 1)
		signal.Notify(interruptChannel, os.Interrupt)

		for range interruptChannel {
			log.Info("Shutting down")
			plugin.CleanupClients()
			os.Exit(0)
		}
	}()

	go func() {
		errChan := make(chan error)
		go func() {
			errChan <- s.TailscaleServer.Start()
		}()

		select {
		case err := <-errChan:
			errCh <- err
		case <-time.After(1 * time.Second):
			go func() {
				errChan <- s.TailscaleServer.Connect()
			}()
		}

		if err := <-errChan; err != nil {
			errCh <- err
		}
	}()

	return nil
}

func (s *Server) GetApiUrl() string {
	return util.GetFrpcApiUrl(s.config.Frps.Protocol, s.config.Id, s.config.Frps.Domain)
}
