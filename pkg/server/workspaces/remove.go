// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package workspaces

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

func (s *WorkspaceService) RemoveWorkspace(workspaceId string) error {
	workspace, err := s.workspaceStore.Find(workspaceId)
	if err != nil {
		return ErrWorkspaceNotFound
	}

	log.Infof("Destroying workspace %s", workspace.Id)

	target, err := s.targetStore.Find(workspace.Target)
	if err != nil {
		return err
	}

	for _, project := range workspace.Projects {
		//	todo: go routines
		err := s.provisioner.DestroyProject(project, target)
		if err != nil {
			return err
		}
	}

	err = s.provisioner.DestroyWorkspace(workspace, target)
	if err != nil {
		return err
	}

	for _, project := range workspace.Projects {
		err := s.apiKeyService.Revoke(fmt.Sprintf("%s/%s", workspace.Id, project.Name))
		if err != nil {
			// Should not fail the whole operation if the API key cannot be revoked
			log.Error(err)
		}
		projectLogger := s.newProjectLogger(workspace.Id, project.Name)
		err = projectLogger.Cleanup()
		if err != nil {
			// Should not fail the whole operation if the project logger cannot be cleaned up
			log.Error(err)
		}
	}

	logger := s.newWorkspaceLogger(workspace.Id)
	err = logger.Cleanup()
	if err != nil {
		// Should not fail the whole operation if the workspace logger cannot be cleaned up
		log.Error(err)
	}

	err = s.workspaceStore.Delete(workspace)
	if err != nil {
		return err
	}

	log.Infof("Workspace %s destroyed", workspace.Id)
	return nil
}
