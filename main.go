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

	// Основная логика валидации
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
	// Валидация верхнего уровня
	v.validateTopLevel(root)

	if len(v.errors) > 0 {
		for _, err := range v.errors {
			fmt.Fprintln(os.Stderr, err)
		}
		return fmt.Errorf("validation failed")
	}
	return nil
}

func (v *Validator) validateTopLevel(root *yaml.Node) {
	if len(root.Content) == 0 {
		v.Errorf(0, "empty document")
		return
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		v.Errorf(doc.Line, "root must be mapping")
		return
	}

	// Проверяем обязательные поля верхнего уровня
	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(doc.Content); i += 2 {
		key := doc.Content[i]
		value := doc.Content[i+1]
		fields[key.Value] = value
	}

	// apiVersion
	if node, exists := fields["apiVersion"]; !exists {
		v.Errorf(0, "apiVersion is required")
	} else {
		v.validateAPIVersion(node)
	}

	// kind
	if node, exists := fields["kind"]; !exists {
		v.Errorf(0, "kind is required")
	} else {
		v.validateKind(node)
	}

	// metadata
	if node, exists := fields["metadata"]; !exists {
		v.Errorf(0, "metadata is required")
	} else {
		v.validateMetadata(node)
	}

	// spec
	if node, exists := fields["spec"]; !exists {
		v.Errorf(0, "spec is required")
	} else {
		v.validateSpec(node)
	}
}

func (v *Validator) validateAPIVersion(node *yaml.Node) {
	if node.Value != "v1" {
		v.Errorf(node.Line, "apiVersion has unsupported value '%s'", node.Value)
	}
}

func (v *Validator) validateKind(node *yaml.Node) {
	if node.Value != "Pod" {
		v.Errorf(node.Line, "kind has unsupported value '%s'", node.Value)
	}
}

func (v *Validator) validateMetadata(node *yaml.Node) {
	if node.Kind != yaml.MappingNode {
		v.Errorf(node.Line, "metadata must be mapping")
		return
	}

	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		fields[key.Value] = value
	}

	// name
	if nameNode, exists := fields["name"]; !exists {
		v.Errorf(0, "metadata.name is required")
	} else if nameNode.Kind != yaml.ScalarNode {
		v.Errorf(nameNode.Line, "name must be string")
	} else if nameNode.Value == "" {
		v.Errorf(nameNode.Line, "name is required")
	}

	// namespace (optional)
	if namespaceNode, exists := fields["namespace"]; exists {
		if namespaceNode.Kind != yaml.ScalarNode {
			v.Errorf(namespaceNode.Line, "namespace must be string")
		}
	}

	// labels (optional)
	if labelsNode, exists := fields["labels"]; exists {
		v.validateLabels(labelsNode)
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

	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		fields[key.Value] = value
	}

	// os (optional)
	if osNode, exists := fields["os"]; exists {
		v.validateOS(osNode)
	}

	// containers (required)
	if containersNode, exists := fields["containers"]; !exists {
		v.Errorf(0, "spec.containers is required")
	} else {
		v.validateContainers(containersNode)
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

		fields := make(map[string]*yaml.Node)
		for i := 0; i < len(containerNode.Content); i += 2 {
			key := containerNode.Content[i]
			value := containerNode.Content[i+1]
			fields[key.Value] = value
		}

		// name
		if nameNode, exists := fields["name"]; !exists {
			v.Errorf(0, "container name is required")
		} else {
			v.validateContainerName(nameNode, containerNames)
		}

		// image
		if imageNode, exists := fields["image"]; !exists {
			v.Errorf(0, "container image is required")
		} else {
			v.validateImage(imageNode)
		}

		// resources
		if resourcesNode, exists := fields["resources"]; !exists {
			v.Errorf(0, "container resources is required")
		} else {
			v.validateResources(resourcesNode)
		}

		// ports (optional)
		if portsNode, exists := fields["ports"]; exists {
			v.validatePorts(portsNode)
		}

		// readinessProbe (optional)
		if probeNode, exists := fields["readinessProbe"]; exists {
			v.validateProbe(probeNode)
		}

		// livenessProbe (optional)
		if probeNode, exists := fields["livenessProbe"]; exists {
			v.validateProbe(probeNode)
		}
	}
}

func (v *Validator) validateContainerName(node *yaml.Node, names map[string]bool) {
	if node.Kind != yaml.ScalarNode {
		v.Errorf(node.Line, "container name must be string")
		return
	}

	// Проверка на пустую строку
	if node.Value == "" {
		v.Errorf(node.Line, "name is required")
		return
	}

	// Проверка формата snake_case
	snakeCaseRegex := regexp.MustCompile(`^[a-z]+(_[a-z]+)*$`)
	if !snakeCaseRegex.MatchString(node.Value) {
		v.Errorf(node.Line, "container name has invalid format '%s'", node.Value)
		return
	}

	// Проверка уникальности
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

	// Проверка формата registry.bigbrother.io/name:tag
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

	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		fields[key.Value] = value
	}

	// requests (optional)
	if requestsNode, exists := fields["requests"]; exists {
		v.validateResourceRequirements(requestsNode, "requests")
	}

	// limits (optional)
	if limitsNode, exists := fields["limits"]; exists {
		v.validateResourceRequirements(limitsNode, "limits")
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
			v.validateCPU(value, prefix)
		case "memory":
			v.validateMemory(value, prefix)
		default:
			v.Errorf(key.Line, "%s.%s has unsupported resource type", prefix, key.Value)
		}
	}
}

func (v *Validator) validateCPU(node *yaml.Node, prefix string) {
	if node.Kind != yaml.ScalarNode {
		v.Errorf(node.Line, "%s.cpu must be integer", prefix)
		return
	}

	if _, err := strconv.Atoi(node.Value); err != nil {
		v.Errorf(node.Line, "%s.cpu must be integer", prefix)
	}
}

func (v *Validator) validateMemory(node *yaml.Node, prefix string) {
	if node.Kind != yaml.ScalarNode {
		v.Errorf(node.Line, "%s.memory must be string", prefix)
		return
	}

	// Проверка формата памяти (например: "500Mi", "1Gi")
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

		fields := make(map[string]*yaml.Node)
		for i := 0; i < len(portNode.Content); i += 2 {
			key := portNode.Content[i]
			value := portNode.Content[i+1]
			fields[key.Value] = value
		}

		// containerPort (required)
		if portNode, exists := fields["containerPort"]; !exists {
			v.Errorf(0, "containerPort is required")
		} else {
			v.validateContainerPort(portNode)
		}

		// protocol (optional)
		if protocolNode, exists := fields["protocol"]; exists {
			v.validateProtocol(protocolNode)
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

	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		fields[key.Value] = value
	}

	// httpGet (required)
	if httpGetNode, exists := fields["httpGet"]; !exists {
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

	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		fields[key.Value] = value
	}

	// path (required)
	if pathNode, exists := fields["path"]; !exists {
		v.Errorf(0, "path is required")
	} else {
		if pathNode.Kind != yaml.ScalarNode {
			v.Errorf(pathNode.Line, "path must be string")
		} else if len(pathNode.Value) == 0 || pathNode.Value[0] != '/' {
			v.Errorf(pathNode.Line, "path must be absolute path")
		}
	}

	// port (required)
	if portNode, exists := fields["port"]; !exists {
		v.Errorf(0, "port is required")
	} else {
		v.validateProbePort(portNode)
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