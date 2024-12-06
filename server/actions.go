package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

type actionDefinitions map[string][]string

func createActionDefinitionsFromJSON(data []byte) (actionDefinitions, error) {
	var definitions actionDefinitions
	if err := json.Unmarshal(data, &definitions); err != nil {
		return nil, fmt.Errorf("can't unmarshal: %w, %s", err, data)
	}
	return definitions, nil
}

func (defs actionDefinitions) actions() ([]string, error) {
	actions, ok := defs["actions"]
	if !ok {
		return nil, errors.New("no actions specified")
	}
	return actions, nil
}

func (defs actionDefinitions) commandFromAction(action string) (*model.Command, error) {
	tokens := strings.Split(action, " ")
	if len(tokens) < 1 {
		return nil, errors.New("definition is empty")
	}

	autocompleteData := model.NewAutocompleteData(tokens[0], "", "")
	for _, token := range tokens[1:] {
		autocompleteData.AddDynamicListArgument(token, "/plugins/com.mattermost.demo-dynamic-slash-commands", true)
	}

	return &model.Command{
		Trigger:          tokens[0],
		AutoComplete:     true,
		AutocompleteData: autocompleteData,
	}, nil
}
