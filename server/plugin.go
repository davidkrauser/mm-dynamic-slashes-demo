package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
)

const externalActionServer = "http://localhost:3000"

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration
}

func updateSlashActions(client *pluginapi.Client, etag *string) error {
	req, err := http.NewRequest("GET", externalActionServer+"/list-actions", nil)
	if err != nil {
		return fmt.Errorf("can't create req: %w", err)
	}
	req.Header.Add("If-None-Match", *etag)

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("can't get: %w", err)
	}
	defer resp.Body.Close()
	*etag = resp.Header.Get("Etag")

	// nothing to do
	if resp.StatusCode == 304 {
		client.Log.Info("Slash actions already up to date")
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("can't read: %w", err)
	}

	definitions, err := createActionDefinitionsFromJSON(body)
	if err != nil {
		return err
	}

	actions, err := definitions.actions()
	if err != nil {
		return err
	}

	for _, action := range actions {
		command, err := definitions.commandFromAction(action)
		if err != nil {
			return err
		}
		if err := client.SlashCommand.Register(command); err != nil {
			return err
		}
	}

	client.Log.Info("Slash actions updated")

	return nil
}

// OnActivate is invoked when the plugin is activated.
func (p *Plugin) OnActivate() error {
	var etag string
	client := pluginapi.NewClient(p.API, p.Driver)
	_, err := cluster.Schedule(p.API, "UpdateSlashActions", cluster.MakeWaitForInterval(2*time.Second), func() {
		p.API.LogInfo("Updating slash actions")
		if err := updateSlashActions(client, &etag); err != nil {
			p.API.LogError("error updating slash actions", "error", err)
		}
	})
	return err
}

// ExecuteCommand executes a command that has been previously registered via the RegisterCommand API.
func (p *Plugin) ExecuteCommand(_ *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	body := make(map[string]string)
	body["action"] = args.Command
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error building request: " + err.Error(),
		}, nil
	}

	resp, err := http.Post(externalActionServer+"/perform-action", "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error sending request: " + err.Error(),
		}, nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error reading request response: " + err.Error(),
		}, nil
	}
	defer resp.Body.Close()

	var commandResponse model.CommandResponse
	if err := json.Unmarshal(respBody, &commandResponse); err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error parsing request response: " + err.Error(),
		}, nil
	}

	return &commandResponse, nil
}
