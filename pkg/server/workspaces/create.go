// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package workspaces

import (
	"fmt"
	"io"
	"regexp"

	"github.com/daytonaio/daytona/pkg/apikey"
	"github.com/daytonaio/daytona/pkg/provider"
	"github.com/daytonaio/daytona/pkg/server/workspaces/dto"
	"github.com/daytonaio/daytona/pkg/workspace"
)

func (s *WorkspaceService) CreateWorkspace(req dto.CreateWorkspaceRequest) (*workspace.Workspace, error) {
	_, err := s.workspaceStore.Find(req.Name)
	if err == nil {
		return nil, ErrWorkspaceAlreadyExists
	}

	isAlphaNumeric := regexp.MustCompile(`^[a-zA-Z0-9-]+$`).MatchString
	if !isAlphaNumeric(req.Name) {
		return nil, ErrInvalidWorkspaceName
	}

	w := &workspace.Workspace{
		Id:     req.Id,
		Name:   req.Name,
		Target: req.Target,
	}

	w.Projects = []*workspace.Project{}

	for _, project := range req.Projects {
		apiKey, err := s.apiKeyService.Generate(apikey.ApiKeyTypeProject, fmt.Sprintf("%s/%s", w.Id, project.Name))
		if err != nil {
			return nil, err
		}

		projectImage := s.defaultProjectImage
		if project.Image != nil {
			projectImage = *project.Image
		}

		project := &workspace.Project{
			Name:        project.Name,
			Image:       projectImage,
			Repository:  project.Source.Repository,
			WorkspaceId: w.Id,
			ApiKey:      apiKey,
			Target:      w.Target,
			EnvVars:     project.EnvVars,
		}
		w.Projects = append(w.Projects, project)
	}

	err = s.workspaceStore.Save(w)
	if err != nil {
		return nil, err
	}

	return s.createWorkspace(w)
}

func (s *WorkspaceService) createProject(project *workspace.Project, target *provider.ProviderTarget, logWriter io.Writer) error {
	logWriter.Write([]byte(fmt.Sprintf("Creating project %s\n", project.Name)))

	projectToCreate := *project
	projectToCreate.EnvVars = workspace.GetProjectEnvVars(project, s.serverApiUrl, s.serverUrl)

	for k, v := range project.EnvVars {
		projectToCreate.EnvVars[k] = v
	}

	err := s.provisioner.CreateProject(&projectToCreate, target)
	if err != nil {
		return err
	}

	logWriter.Write([]byte(fmt.Sprintf("Project %s created\n", project.Name)))

	return nil
}

func (s *WorkspaceService) createWorkspace(workspace *workspace.Workspace) (*workspace.Workspace, error) {
	target, err := s.targetStore.Find(workspace.Target)
	if err != nil {
		return workspace, err
	}

	wsLogger := s.newWorkspaceLogger(workspace.Id)
	wsLogger.Write([]byte("Creating workspace\n"))

	err = s.provisioner.CreateWorkspace(workspace, target)
	if err != nil {
		return nil, err
	}

	for _, project := range workspace.Projects {
		projectLogger := s.newProjectLogger(workspace.Id, project.Name)
		defer projectLogger.Close()

		projectLogWriter := io.MultiWriter(wsLogger, projectLogger)
		err := s.createProject(project, target, projectLogWriter)
		if err != nil {
			return nil, err
		}
	}

	wsLogger.Write([]byte("Workspace creation complete. Pending start...\n"))

	err = s.startWorkspace(workspace, target, wsLogger)
	if err != nil {
		return nil, err
	}

	return workspace, nil
}
