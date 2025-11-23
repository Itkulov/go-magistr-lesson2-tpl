package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type ValidationError struct {
	File    string
	Line    int
	Message string
}

func (e ValidationError) String() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d %s", e.File, e.Line, e.Message)
	}
	return fmt.Sprintf("%s %s", e.File, e.Message)
}

var errors []ValidationError

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: yamlvalid <path-to-yaml-file>\n")
		os.Exit(1)
	}

	filePath := os.Args[1]

	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s cannot read file\n", filePath)
		os.Exit(1)
	}

	// Parse YAML
	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		fmt.Fprintf(os.Stderr, "%s invalid yaml format\n", filePath)
		os.Exit(1)
	}

	// Validate
	validatePod(&root, filePath)

	// Output errors
	if len(errors) > 0 {
		for _, e := range errors {
			fmt.Fprintf(os.Stderr, "%s\n", e)
		}
		os.Exit(1)
	}

	os.Exit(0)
}

func validatePod(root *yaml.Node, filePath string) {
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return
	}

	pod := parseMapping(doc)

	// Validate top-level fields
	if !hasField(pod, "apiVersion") {
		errors = append(errors, ValidationError{File: filePath, Message: "apiVersion is required"})
	} else if apiVersion := getStringValue(pod, "apiVersion"); apiVersion != "v1" {
		errors = append(errors, ValidationError{
			File: filePath,
			Line: getFieldLine(pod, "apiVersion"),
			Message: fmt.Sprintf("apiVersion has unsupported value '%s'", apiVersion),
		})
	}

	if !hasField(pod, "kind") {
		errors = append(errors, ValidationError{File: filePath, Message: "kind is required"})
	} else if kind := getStringValue(pod, "kind"); kind != "Pod" {
		errors = append(errors, ValidationError{
			File: filePath,
			Line: getFieldLine(pod, "kind"),
			Message: fmt.Sprintf("kind has unsupported value '%s'", kind),
		})
	}

	if !hasField(pod, "metadata") {
		errors = append(errors, ValidationError{File: filePath, Message: "metadata is required"})
	} else {
		validateMetadata(pod["metadata"], filePath)
	}

	if !hasField(pod, "spec") {
		errors = append(errors, ValidationError{File: filePath, Message: "spec is required"})
	} else {
		validateSpec(pod["spec"], filePath)
	}
}

func validateMetadata(node *yaml.Node, filePath string) {
	if node.Kind != yaml.MappingNode {
		return
	}

	metadata := parseMapping(node)

	if !hasField(metadata, "name") {
		errors = append(errors, ValidationError{File: filePath, Message: "metadata.name is required"})
	} else {
		name := getStringValue(metadata, "name")
		if name == "" {
			errors = append(errors, ValidationError{
				File: filePath,
				Line: getFieldLine(metadata, "name"),
				Message: "metadata.name is required",
			})
		}
	}
}

func validateSpec(node *yaml.Node, filePath string) {
	if node.Kind != yaml.MappingNode {
		return
	}

	spec := parseMapping(node)

	// Validate OS if present
	if hasField(spec, "os") {
		validateOS(spec["os"], filePath)
	}

	// Validate containers
	if !hasField(spec, "containers") {
		errors = append(errors, ValidationError{File: filePath, Message: "spec.containers is required"})
	} else {
		validateContainers(spec["containers"], filePath)
	}
}

func validateOS(node *yaml.Node, filePath string) {
	if node.Kind != yaml.MappingNode {
		return
	}

	osMap := parseMapping(node)

	if !hasField(osMap, "name") {
		errors = append(errors, ValidationError{
			File: filePath,
			Line: node.Line,
			Message: "os.name is required",
		})
	} else {
		osName := getStringValue(osMap, "name")
		if osName != "linux" && osName != "windows" {
			errors = append(errors, ValidationError{
				File: filePath,
				Line: getFieldLine(osMap, "name"),
				Message: fmt.Sprintf("os.name has unsupported value '%s'", osName),
			})
		}
	}
}

func validateContainers(node *yaml.Node, filePath string) {
	if node.Kind != yaml.SequenceNode {
		errors = append(errors, ValidationError{
			File: filePath,
			Line: node.Line,
			Message: "spec.containers must be a list",
		})
		return
	}

	containerNames := make(map[string]bool)

	for _, containerNode := range node.Content {
		if containerNode.Kind != yaml.MappingNode {
			continue
		}

		container := parseMapping(containerNode)

		// Validate name
		if !hasField(container, "name") {
			errors = append(errors, ValidationError{
				File: filePath,
				Line: containerNode.Line,
				Message: "containers.name is required",
			})
		} else {
			name := getStringValue(container, "name")
			if !isValidSnakeCase(name) {
				errors = append(errors, ValidationError{
					File: filePath,
					Line: getFieldLine(container, "name"),
					Message: fmt.Sprintf("containers.name has invalid format '%s'", name),
				})
			}
			if containerNames[name] {
				errors = append(errors, ValidationError{
					File: filePath,
					Line: getFieldLine(container, "name"),
					Message: fmt.Sprintf("containers.name must be unique, '%s' is duplicated", name),
				})
			}
			containerNames[name] = true
		}

		// Validate image
		if !hasField(container, "image") {
			errors = append(errors, ValidationError{
				File: filePath,
				Line: containerNode.Line,
				Message: "containers.image is required",
			})
		} else {
			image := getStringValue(container, "image")
			validateImage(image, getFieldLine(container, "image"), filePath)
		}

		// Validate ports if present
		if hasField(container, "ports") {
			validatePorts(container["ports"], filePath)
		}

		// Validate probes if present
		if hasField(container, "readinessProbe") {
			validateProbe(container["readinessProbe"], "readinessProbe", filePath)
		}
		if hasField(container, "livenessProbe") {
			validateProbe(container["livenessProbe"], "livenessProbe", filePath)
		}

		// Validate resources
		if !hasField(container, "resources") {
			errors = append(errors, ValidationError{
				File: filePath,
				Line: containerNode.Line,
				Message: "containers.resources is required",
			})
		} else {
			validateResources(container["resources"], filePath)
		}
	}
}

func validateImage(image string, line int, filePath string) {
	if !strings.HasPrefix(image, "registry.bigbrother.io/") {
		errors = append(errors, ValidationError{
			File: filePath,
			Line: line,
			Message: fmt.Sprintf("containers.image has invalid format '%s'", image),
		})
		return
	}

	if !strings.Contains(image[len("registry.bigbrother.io/"):], ":") {
		errors = append(errors, ValidationError{
			File: filePath,
			Line: line,
			Message: fmt.Sprintf("containers.image has invalid format '%s'", image),
		})
	}
}

func validatePorts(node *yaml.Node, filePath string) {
	if node.Kind != yaml.SequenceNode {
		return
	}

	for _, portNode := range node.Content {
		if portNode.Kind != yaml.MappingNode {
			continue
		}

		port := parseMapping(portNode)

		if !hasField(port, "containerPort") {
			errors = append(errors, ValidationError{
				File: filePath,
				Line: portNode.Line,
				Message: "containers.ports.containerPort is required",
			})
		} else {
			portValue := getIntValue(port, "containerPort")
			if portValue <= 0 || portValue > 65535 {
				errors = append(errors, ValidationError{
					File: filePath,
					Line: getFieldLine(port, "containerPort"),
					Message: "containers.ports.containerPort value out of range",
				})
			}
		}

		if hasField(port, "protocol") {
			protocol := getStringValue(port, "protocol")
			if protocol != "TCP" && protocol != "UDP" {
				errors = append(errors, ValidationError{
					File: filePath,
					Line: getFieldLine(port, "protocol"),
					Message: fmt.Sprintf("containers.ports.protocol has unsupported value '%s'", protocol),
				})
			}
		}
	}
}

func validateProbe(node *yaml.Node, probeName string, filePath string) {
	if node.Kind != yaml.MappingNode {
		return
	}

	probe := parseMapping(node)

	if !hasField(probe, "httpGet") {
		errors = append(errors, ValidationError{
			File: filePath,
			Line: node.Line,
			Message: fmt.Sprintf("containers.%s.httpGet is required", probeName),
		})
	} else {
		validateHTTPGetAction(probe["httpGet"], probeName, filePath)
	}
}

func validateHTTPGetAction(node *yaml.Node, probeName string, filePath string) {
	if node.Kind != yaml.MappingNode {
		return
	}

	action := parseMapping(node)

	if !hasField(action, "path") {
		errors = append(errors, ValidationError{
			File: filePath,
			Line: node.Line,
			Message: fmt.Sprintf("containers.%s.httpGet.path is required", probeName),
		})
	} else {
		path := getStringValue(action, "path")
		if !strings.HasPrefix(path, "/") {
			errors = append(errors, ValidationError{
				File: filePath,
				Line: getFieldLine(action, "path"),
				Message: fmt.Sprintf("containers.%s.httpGet.path has invalid format '%s'", probeName, path),
			})
		}
	}

	if !hasField(action, "port") {
		errors = append(errors, ValidationError{
			File: filePath,
			Line: node.Line,
			Message: fmt.Sprintf("containers.%s.httpGet.port is required", probeName),
		})
	} else {
		portValue := getIntValue(action, "port")
		if portValue <= 0 || portValue > 65535 {
			errors = append(errors, ValidationError{
				File: filePath,
				Line: getFieldLine(action, "port"),
				Message: fmt.Sprintf("containers.%s.httpGet.port value out of range", probeName),
			})
		}
	}
}

func validateResources(node *yaml.Node, filePath string) {
	if node.Kind != yaml.MappingNode {
		return
	}

	resources := parseMapping(node)

	if hasField(resources, "requests") {
		validateResourceSpec(resources["requests"], "requests", filePath)
	}

	if hasField(resources, "limits") {
		validateResourceSpec(resources["limits"], "limits", filePath)
	}
}

func validateResourceSpec(node *yaml.Node, specName string, filePath string) {
	if node.Kind != yaml.MappingNode {
		return
	}

	spec := parseMapping(node)

	if hasField(spec, "cpu") {
		cpuValue := getIntValue(spec, "cpu")
		if cpuValue <= 0 {
			errors = append(errors, ValidationError{
				File: filePath,
				Line: getFieldLine(spec, "cpu"),
				Message: "containers.resources." + specName + ".cpu value out of range",
			})
		}
	}

	if hasField(spec, "memory") {
		memory := getStringValue(spec, "memory")
		if !isValidMemoryFormat(memory) {
			errors = append(errors, ValidationError{
				File: filePath,
				Line: getFieldLine(spec, "memory"),
				Message: fmt.Sprintf("containers.resources.%s.memory has invalid format '%s'", specName, memory),
			})
		}
	}
}

// Helper functions

func parseMapping(node *yaml.Node) map[string]*yaml.Node {
	result := make(map[string]*yaml.Node)
	if node.Kind != yaml.MappingNode {
		return result
	}

	for i := 0; i < len(node.Content); i += 2 {
		if i+1 < len(node.Content) {
			key := node.Content[i].Value
			result[key] = node.Content[i+1]
		}
	}
	return result
}

func hasField(m map[string]*yaml.Node, key string) bool {
	_, ok := m[key]
	return ok
}

func getStringValue(m map[string]*yaml.Node, key string) string {
	if node, ok := m[key]; ok {
		return node.Value
	}
	return ""
}

func getIntValue(m map[string]*yaml.Node, key string) int {
	if node, ok := m[key]; ok {
		val, _ := strconv.Atoi(node.Value)
		return val
	}
	return 0
}

func getFieldLine(m map[string]*yaml.Node, key string) int {
	if node, ok := m[key]; ok {
		return node.Line
	}
	return 0
}

func isValidSnakeCase(s string) bool {
	// snake_case: только буквы, цифры и подчёркивания
	matched, _ := regexp.MatchString(`^[a-z0-9_]+$`, s)
	return matched && s != ""
}

func isValidMemoryFormat(s string) bool {
	// Должно заканчиваться на Gi, Mi, Ki и начинаться с цифры
	matched, _ := regexp.MatchString(`^\d+(Gi|Mi|Ki)$`, s)
	return matched
}
