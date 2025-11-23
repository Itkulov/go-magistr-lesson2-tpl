package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <yaml-file>\n", os.Args[0])
		os.Exit(1)
	}

	filename := os.Args[1]
	if err := validateYAML(filename); err != nil {
		os.Exit(1)
	}
}

func validateYAML(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read file content: %v\n", err)
		return err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		fmt.Fprintf(os.Stderr, "cannot unmarshal file content: %v\n", err)
		return err
	}

	validator := NewValidator(filename)
	return validator.Validate(&root)
}

type Validator struct {
	filename string
	errors   []string
}

func NewValidator(filename string) *Validator {
	return &Validator{
		filename: filename,
		errors:   make([]string, 0),
	}
}

func (v *Validator) Errorf(line int, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if line > 0 {
		v.errors = append(v.errors, fmt.Sprintf("%s:%d %s", v.filename, line, msg))
	} else {
		v.errors = append(v.errors, fmt.Sprintf("%s %s", v.filename, msg))
	}
}

func (v *Validator) Validate(root *yaml.Node) error {
	if len(root.Content) == 0 {
		v.Errorf(0, "empty document")
	}

	doc := root.Content[0]
	v.validateDocument(doc)

	if len(v.errors) > 0 {
		for _, err := range v.errors {
			fmt.Fprintln(os.Stderr, err)
		}
		return fmt.Errorf("validation failed")
	}
	return nil
}

func (v *Validator) validateDocument(node *yaml.Node) {
	if node.Kind != yaml.MappingNode {
		v.Errorf(node.Line, "root must be mapping")
		return
	}

	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]

		switch key.Value {
		case "apiVersion":
			v.validateAPIVersion(value)
		case "kind":
			v.validateKind(value)
		case "metadata":
			v.validateMetadata(value)
		case "spec":
			v.validateSpec(value)
		}
	}
}

func (v *Validator) validateAPIVersion(node *yaml.Node) {
	if node.Kind != yaml.ScalarNode {
		v.Errorf(node.Line, "apiVersion must be string")
		return
	}
	if node.Value != "v1" {
		v.Errorf(node.Line, "apiVersion has unsupported value '%s'", node.Value)
	}
}

func (v *Validator) validateKind(node *yaml.Node) {
	if node.Kind != yaml.ScalarNode {
		v.Errorf(node.Line, "kind must be string")
		return
	}
	if node.Value != "Pod" {
		v.Errorf(node.Line, "kind has unsupported value '%s'", node.Value)
	}
}

func (v *Validator) validateMetadata(node *yaml.Node) {
	if node.Kind != yaml.MappingNode {
		v.Errorf(node.Line, "metadata must be mapping")
		return
	}

	var nameNode *yaml.Node
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]

		switch key.Value {
		case "name":
			nameNode = value
		case "namespace":
			if value.Kind != yaml.ScalarNode {
				v.Errorf(value.Line, "namespace must be string")
			}
		case "labels":
			v.validateLabels(value)
		}
	}

	if nameNode == nil {
		v.Errorf(0, "metadata.name is required")
	} else {
		v.validateName(nameNode)
	}
}

func (v *Validator) validateName(node *yaml.Node) {
	if node.Kind != yaml.ScalarNode {
		v.Errorf(node.Line, "name must be string")
		return
	}
	if node.Value == "" {
		v.Errorf(node.Line, "name is required")
	}
}

func (v *Validator) validateLabels(node *yaml.Node) {
	if node.Kind != yaml.MappingNode {
		v.Errorf(node.Line, "labels must be mapping")
		return
	}

	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]

		if key.Kind != yaml.ScalarNode {
			v.Errorf(key.Line, "label key must be string")
		}
		if value.Kind != yaml.ScalarNode {
			v.Errorf(value.Line, "label value must be string")
		}
	}
}

func (v *Validator) validateSpec(node *yaml.Node) {
	if node.Kind != yaml.MappingNode {
		v.Errorf(node.Line, "spec must be mapping")
		return
	}

	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]

		switch key.Value {
		case "os":
			v.validateOS(value)
		case "containers":
			v.validateContainers(value)
		}
	}
}

func (v *Validator) validateOS(node *yaml.Node) {
	if node.Kind != yaml.ScalarNode {
		v.Errorf(node.Line, "os must be string")
		return
	}
	if node.Value != "linux" && node.Value != "windows" {
		v.Errorf(node.Line, "os has unsupported value '%s'", node.Value)
	}
}

func (v *Validator) validateContainers(node *yaml.Node) {
	if node.Kind != yaml.SequenceNode {
		v.Errorf(node.Line, "containers must be sequence")
		return
	}

	containerNames := make(map[string]bool)
	for _, containerNode := range node.Content {
		if containerNode.Kind != yaml.MappingNode {
			v.Errorf(containerNode.Line, "container must be mapping")
			continue
		}
		v.validateContainer(containerNode, containerNames)
	}
}

func (v *Validator) validateContainer(node *yaml.Node, names map[string]bool) {
	var nameNode, imageNode, resourcesNode *yaml.Node

	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]

		switch key.Value {
		case "name":
			nameNode = value
		case "image":
			imageNode = value
		case "resources":
			resourcesNode = value
		case "ports":
			v.validatePorts(value)
		case "readinessProbe":
			v.validateProbe(value)
		case "livenessProbe":
			v.validateProbe(value)
		}
	}

	if nameNode == nil {
		v.Errorf(0, "container name is required")
	} else {
		v.validateContainerName(nameNode, names)
	}

	if imageNode == nil {
		v.Errorf(0, "container image is required")
	} else {
		v.validateImage(imageNode)
	}

	if resourcesNode == nil {
		v.Errorf(0, "container resources is required")
	} else {
		v.validateResources(resourcesNode)
	}
}

func (v *Validator) validateContainerName(node *yaml.Node, names map[string]bool) {
	if node.Kind != yaml.ScalarNode {
		v.Errorf(node.Line, "container name must be string")
		return
	}
	if node.Value == "" {
		v.Errorf(node.Line, "name is required")
		return
	}

	snakeCaseRegex := regexp.MustCompile(`^[a-z]+(_[a-z]+)*$`)
	if !snakeCaseRegex.MatchString(node.Value) {
		v.Errorf(node.Line, "container name has invalid format '%s'", node.Value)
		return
	}

	if names[node.Value] {
		v.Errorf(node.Line, "container name '%s' is not unique", node.Value)
	} else {
		names[node.Value] = true
	}
}

func (v *Validator) validateImage(node *yaml.Node) {
	if node.Kind != yaml.ScalarNode {
		v.Errorf(node.Line, "image must be string")
		return
	}

	imageRegex := regexp.MustCompile(`^registry\.bigbrother\.io/[a-zA-Z0-9][a-zA-Z0-9_.-]+:[a-zA-Z0-9_.-]+$`)
	if !imageRegex.MatchString(node.Value) {
		v.Errorf(node.Line, "image has invalid format '%s'", node.Value)
	}
}

func (v *Validator) validateResources(node *yaml.Node) {
	if node.Kind != yaml.MappingNode {
		v.Errorf(node.Line, "resources must be mapping")
		return
	}

	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]

		switch key.Value {
		case "requests":
			v.validateResourceRequirements(value, "requests")
		case "limits":
			v.validateResourceRequirements(value, "limits")
		}
	}
}

func (v *Validator) validateResourceRequirements(node *yaml.Node, prefix string) {
	if node.Kind != yaml.MappingNode {
		v.Errorf(node.Line, "%s must be mapping", prefix)
		return
	}

	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]

		switch key.Value {
		case "cpu":
			v.validateCPU(value)
		case "memory":
			v.validateMemory(value, prefix)
		}
	}
}

func (v *Validator) validateCPU(node *yaml.Node) {
	if node.Kind != yaml.ScalarNode {
		v.Errorf(node.Line, "cpu must be integer")
		return
	}
	if _, err := strconv.Atoi(node.Value); err != nil {
		v.Errorf(node.Line, "cpu must be int")
	}
}

func (v *Validator) validateMemory(node *yaml.Node, prefix string) {
	if node.Kind != yaml.ScalarNode {
		v.Errorf(node.Line, "%s.memory must be string", prefix)
		return
	}

	memoryRegex := regexp.MustCompile(`^\d+(Gi|Mi|Ki)$`)
	if !memoryRegex.MatchString(node.Value) {
		v.Errorf(node.Line, "%s.memory has invalid format '%s'", prefix, node.Value)
	}
}

func (v *Validator) validatePorts(node *yaml.Node) {
	if node.Kind != yaml.SequenceNode {
		v.Errorf(node.Line, "ports must be sequence")
		return
	}

	for _, portNode := range node.Content {
		if portNode.Kind != yaml.MappingNode {
			v.Errorf(portNode.Line, "port must be mapping")
			continue
		}

		var containerPortNode *yaml.Node
		for i := 0; i < len(portNode.Content); i += 2 {
			key := portNode.Content[i]
			value := portNode.Content[i+1]

			switch key.Value {
			case "containerPort":
				containerPortNode = value
			case "protocol":
				v.validateProtocol(value)
			}
		}

		if containerPortNode == nil {
			v.Errorf(0, "containerPort is required")
		} else {
			v.validateContainerPort(containerPortNode)
		}
	}
}

func (v *Validator) validateContainerPort(node *yaml.Node) {
	if node.Kind != yaml.ScalarNode {
		v.Errorf(node.Line, "containerPort must be integer")
		return
	}

	port, err := strconv.Atoi(node.Value)
	if err != nil {
		v.Errorf(node.Line, "containerPort must be integer")
		return
	}

	if port <= 0 || port >= 65536 {
		v.Errorf(node.Line, "containerPort value out of range")
	}
}

func (v *Validator) validateProtocol(node *yaml.Node) {
	if node.Kind != yaml.ScalarNode {
		v.Errorf(node.Line, "protocol must be string")
		return
	}
	if node.Value != "TCP" && node.Value != "UDP" {
		v.Errorf(node.Line, "protocol has unsupported value '%s'", node.Value)
	}
}

func (v *Validator) validateProbe(node *yaml.Node) {
	if node.Kind != yaml.MappingNode {
		v.Errorf(node.Line, "probe must be mapping")
		return
	}

	var httpGetNode *yaml.Node
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]

		if key.Value == "httpGet" {
			httpGetNode = value
		}
	}

	if httpGetNode == nil {
		v.Errorf(0, "httpGet is required")
	} else {
		v.validateHTTPGetAction(httpGetNode)
	}
}

func (v *Validator) validateHTTPGetAction(node *yaml.Node) {
	if node.Kind != yaml.MappingNode {
		v.Errorf(node.Line, "httpGet must be mapping")
		return
	}

	var pathNode, portNode *yaml.Node
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]

		switch key.Value {
		case "path":
			pathNode = value
		case "port":
			portNode = value
		}
	}

	if pathNode == nil {
		v.Errorf(0, "path is required")
	} else {
		v.validatePath(pathNode)
	}

	if portNode == nil {
		v.Errorf(0, "port is required")
	} else {
		v.validateProbePort(portNode)
	}
}

func (v *Validator) validatePath(node *yaml.Node) {
	if node.Kind != yaml.ScalarNode {
		v.Errorf(node.Line, "path must be string")
		return
	}
	if len(node.Value) == 0 || node.Value[0] != '/' {
		v.Errorf(node.Line, "path must be absolute path")
	}
}

func (v *Validator) validateProbePort(node *yaml.Node) {
	if node.Kind != yaml.ScalarNode {
		v.Errorf(node.Line, "port must be integer")
		return
	}

	port, err := strconv.Atoi(node.Value)
	if err != nil {
		v.Errorf(node.Line, "port must be integer")
		return
	}

	if port <= 0 || port >= 65536 {
		v.Errorf(node.Line, "port value out of range")
	}
}
