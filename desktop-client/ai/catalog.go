package ai

// This file loads immutable prompt versions and resolves configured prompt aliases.

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"reflect"
	"sort"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

//go:embed prompts
var promptFiles embed.FS

// ParameterDefinition declares one value accepted by a catalog prompt.
type ParameterDefinition struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Required    bool   `yaml:"required"`
	Description string `yaml:"description"`
}

// ResponseSettings controls model generation and final response validation.
type ResponseSettings struct {
	Format           string   `yaml:"format"`
	RequiredSections []string `yaml:"required_sections"`
	MaxCharacters    int      `yaml:"max_characters"`
	Temperature      float64  `yaml:"temperature"`
	MaxTokens        int      `yaml:"max_tokens"`
}

// ToolSettings bounds iterative tool execution for one prompt.
type ToolSettings struct {
	MaxRounds int `yaml:"max_rounds"`
	MaxCalls  int `yaml:"max_calls"`
}

// PromptDefinition is one immutable, versioned prompt contract.
type PromptDefinition struct {
	ID             string                `yaml:"id"`
	Version        int                   `yaml:"version"`
	Description    string                `yaml:"description"`
	Parameters     []ParameterDefinition `yaml:"parameters"`
	Tools          []string              `yaml:"tools"`
	ToolSettings   ToolSettings          `yaml:"tool_settings"`
	SystemTemplate string                `yaml:"system_template"`
	UserTemplate   string                `yaml:"user_template"`
	Response       ResponseSettings      `yaml:"response"`
}

// PromptReference identifies one retained prompt version.
type PromptReference struct {
	ID      string `yaml:"id"`
	Version int    `yaml:"version"`
}

type catalogManifest struct {
	Aliases map[string]PromptReference `yaml:"aliases"`
}

type promptKey struct {
	id      string
	version int
}

// Catalog stores retained prompt versions and active aliases.
type Catalog struct {
	prompts map[promptKey]PromptDefinition
	aliases map[string]PromptReference
}

// NewCatalog loads the embedded catalog manifest and all immutable prompt versions.
func NewCatalog() (*Catalog, error) {
	return loadCatalog(promptFiles, "prompts/catalog.yaml", "prompts/*/*.yaml")
}

func (c *Catalog) resolve(alias string) (PromptDefinition, error) {
	reference, ok := c.aliases[alias]
	if !ok {
		return PromptDefinition{}, fmt.Errorf("prompt alias %q is not registered", alias)
	}
	definition, ok := c.prompts[promptKey{id: reference.ID, version: reference.Version}]
	if !ok {
		return PromptDefinition{}, fmt.Errorf("prompt alias %q references missing %s v%d", alias, reference.ID, reference.Version)
	}
	return definition, nil
}

func (c *Catalog) version(id string, version int) (PromptDefinition, error) {
	definition, ok := c.prompts[promptKey{id: id, version: version}]
	if !ok {
		return PromptDefinition{}, fmt.Errorf("prompt %s v%d is not registered", id, version)
	}
	return definition, nil
}

func loadCatalog(source fs.FS, manifestPath, promptPattern string) (*Catalog, error) {
	manifestContents, err := fs.ReadFile(source, manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read prompt catalog manifest: %w", err)
	}
	var manifest catalogManifest
	if err := decodeYAML(manifestContents, &manifest); err != nil {
		return nil, fmt.Errorf("decode prompt catalog manifest: %w", err)
	}
	if len(manifest.Aliases) == 0 {
		return nil, fmt.Errorf("prompt catalog manifest has no aliases")
	}

	paths, err := fs.Glob(source, promptPattern)
	if err != nil {
		return nil, fmt.Errorf("find prompt definitions: %w", err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no prompt definitions found")
	}
	sort.Strings(paths)

	catalog := &Catalog{
		prompts: make(map[promptKey]PromptDefinition, len(paths)),
		aliases: manifest.Aliases,
	}
	for _, path := range paths {
		contents, err := fs.ReadFile(source, path)
		if err != nil {
			return nil, fmt.Errorf("read prompt %s: %w", path, err)
		}
		var definition PromptDefinition
		if err := decodeYAML(contents, &definition); err != nil {
			return nil, fmt.Errorf("decode prompt %s: %w", path, err)
		}
		if err := validateDefinition(definition); err != nil {
			return nil, fmt.Errorf("invalid prompt %s: %w", path, err)
		}
		key := promptKey{id: definition.ID, version: definition.Version}
		if _, exists := catalog.prompts[key]; exists {
			return nil, fmt.Errorf("duplicate prompt %s v%d", definition.ID, definition.Version)
		}
		catalog.prompts[key] = definition
	}

	for alias, reference := range catalog.aliases {
		if strings.TrimSpace(alias) == "" || strings.TrimSpace(reference.ID) == "" || reference.Version < 1 {
			return nil, fmt.Errorf("invalid prompt alias %q", alias)
		}
		if _, err := catalog.version(reference.ID, reference.Version); err != nil {
			return nil, fmt.Errorf("invalid prompt alias %q: %w", alias, err)
		}
	}
	return catalog, nil
}

func decodeYAML(contents []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	return decoder.Decode(target)
}

func validateDefinition(definition PromptDefinition) error {
	if strings.TrimSpace(definition.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if definition.Version < 1 {
		return fmt.Errorf("version must be at least 1")
	}
	if strings.TrimSpace(definition.Description) == "" {
		return fmt.Errorf("description is required")
	}
	if strings.TrimSpace(definition.SystemTemplate) == "" || strings.TrimSpace(definition.UserTemplate) == "" {
		return fmt.Errorf("system_template and user_template are required")
	}
	if definition.Response.MaxCharacters < 1 || definition.Response.MaxTokens < 1 {
		return fmt.Errorf("response limits must be positive")
	}
	if definition.Response.Temperature < 0 || definition.Response.Temperature > 2 {
		return fmt.Errorf("response temperature must be between 0 and 2")
	}
	switch definition.Response.Format {
	case "text", "markdown", "json":
	default:
		return fmt.Errorf("response format must be text, markdown, or json")
	}
	if definition.Response.Format != "markdown" && len(definition.Response.RequiredSections) > 0 {
		return fmt.Errorf("required_sections can only be used with markdown responses")
	}
	for _, section := range definition.Response.RequiredSections {
		if strings.TrimSpace(section) == "" {
			return fmt.Errorf("required response sections must not be empty")
		}
	}
	if len(definition.Tools) > 0 && (definition.ToolSettings.MaxRounds < 1 || definition.ToolSettings.MaxCalls < 1) {
		return fmt.Errorf("tool limits must be positive when tools are declared")
	}

	parameterNames := make(map[string]struct{}, len(definition.Parameters))
	for _, parameter := range definition.Parameters {
		if strings.TrimSpace(parameter.Name) == "" {
			return fmt.Errorf("parameter name is required")
		}
		if strings.TrimSpace(parameter.Description) == "" {
			return fmt.Errorf("parameter %q description is required", parameter.Name)
		}
		if _, exists := parameterNames[parameter.Name]; exists {
			return fmt.Errorf("duplicate parameter %q", parameter.Name)
		}
		if !supportedParameterType(parameter.Type) {
			return fmt.Errorf("parameter %q has unsupported type %q", parameter.Name, parameter.Type)
		}
		parameterNames[parameter.Name] = struct{}{}
	}

	toolNames := make(map[string]struct{}, len(definition.Tools))
	for _, name := range definition.Tools {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("tool name is required")
		}
		if _, exists := toolNames[name]; exists {
			return fmt.Errorf("duplicate tool %q", name)
		}
		toolNames[name] = struct{}{}
	}

	if _, err := parseTemplate(definition.ID+"_system", definition.SystemTemplate); err != nil {
		return fmt.Errorf("parse system_template: %w", err)
	}
	if _, err := parseTemplate(definition.ID+"_user", definition.UserTemplate); err != nil {
		return fmt.Errorf("parse user_template: %w", err)
	}
	return nil
}

func validateParameters(definitions []ParameterDefinition, values map[string]any) error {
	declared := make(map[string]ParameterDefinition, len(definitions))
	for _, definition := range definitions {
		declared[definition.Name] = definition
		value, exists := values[definition.Name]
		if definition.Required && (!exists || value == nil) {
			return fmt.Errorf("required parameter %q is missing", definition.Name)
		}
		if exists && value != nil && !matchesParameterType(value, definition.Type) {
			return fmt.Errorf("parameter %q must be %s", definition.Name, definition.Type)
		}
	}
	for name := range values {
		if _, exists := declared[name]; !exists {
			return fmt.Errorf("parameter %q is not declared", name)
		}
	}
	return nil
}

func renderTemplate(name, contents string, parameters map[string]any) (string, error) {
	parsed, err := parseTemplate(name, contents)
	if err != nil {
		return "", fmt.Errorf("parse prompt template %q: %w", name, err)
	}
	var rendered strings.Builder
	if err := parsed.Execute(&rendered, parameters); err != nil {
		return "", fmt.Errorf("render prompt template %q: %w", name, err)
	}
	return strings.TrimSpace(rendered.String()), nil
}

func parseTemplate(name, contents string) (*template.Template, error) {
	return template.New(name).Option("missingkey=error").Parse(contents)
}

func supportedParameterType(value string) bool {
	switch value {
	case "string", "number", "boolean", "list", "object":
		return true
	default:
		return false
	}
}

func matchesParameterType(value any, expected string) bool {
	kind := reflect.TypeOf(value).Kind()
	switch expected {
	case "string":
		return kind == reflect.String
	case "number":
		return kind >= reflect.Int && kind <= reflect.Float64
	case "boolean":
		return kind == reflect.Bool
	case "list":
		return kind == reflect.Array || kind == reflect.Slice
	case "object":
		return kind == reflect.Map || kind == reflect.Struct
	default:
		return false
	}
}
