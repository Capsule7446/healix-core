package execution

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type InstanceID struct{ value string }

type EntryID struct{ value string }

type InvocationPath struct{ value string }

type StepExecutionID struct{ value string }

func NewInstanceID(value string) (InstanceID, error) {
	validated, err := validateExecutionIdentity("instance id", value)
	return InstanceID{value: validated}, err
}

func NewEntryID(value string) (EntryID, error) {
	validated, err := validateExecutionIdentity("entry id", value)
	return EntryID{value: validated}, err
}

func NewStepExecutionID(value string) (StepExecutionID, error) {
	validated, err := validateExecutionIdentity("step execution id", value)
	return StepExecutionID{value: validated}, err
}

func RootInvocationPath(entry EntryID) InvocationPath {
	return InvocationPath{value: entry.value}
}

func ParseInvocationPath(value string) (InvocationPath, error) {
	if _, err := validateInvocationPath(value); err != nil {
		return InvocationPath{}, err
	}
	return InvocationPath{value: value}, nil
}

func (p InvocationPath) Child(stepID string) (InvocationPath, error) {
	if _, err := validateExecutionIdentity("workflow step id", stepID); err != nil {
		return InvocationPath{}, err
	}
	child := p.value + "/" + strconv.Itoa(len(stepID)) + ":" + stepID
	if _, err := validateInvocationPath(child); err != nil {
		return InvocationPath{}, err
	}
	return InvocationPath{value: child}, nil
}

func (id InstanceID) String() string      { return id.value }
func (id EntryID) String() string         { return id.value }
func (id StepExecutionID) String() string { return id.value }
func (p InvocationPath) String() string   { return p.value }

func (id InstanceID) Validate() error {
	_, err := validateExecutionIdentity("instance id", id.value)
	return err
}
func (id EntryID) Validate() error {
	_, err := validateExecutionIdentity("entry id", id.value)
	return err
}
func (id StepExecutionID) Validate() error {
	_, err := validateExecutionIdentity("step execution id", id.value)
	return err
}
func (p InvocationPath) Validate() error { _, err := validateInvocationPath(p.value); return err }

func validateExecutionIdentity(kind, value string) (string, error) {
	if strings.TrimSpace(value) == "" || len(value) > MaxStringBytes {
		return "", fmt.Errorf("%s is invalid", kind)
	}
	if value != strings.TrimSpace(value) {
		return "", fmt.Errorf("%s must not contain leading or trailing whitespace", kind)
	}
	return value, nil
}

func validateInvocationPath(value string) (string, error) {
	if _, err := validateExecutionIdentity("invocation path", value); err != nil {
		return "", err
	}
	rootEnd := strings.IndexByte(value, '/')
	if rootEnd < 0 {
		return value, nil
	}
	if _, err := validateExecutionIdentity("invocation root", value[:rootEnd]); err != nil {
		return "", err
	}
	position := rootEnd + 1
	for position < len(value) {
		remaining := value[position:]
		separator := strings.IndexByte(remaining, ':')
		if separator < 1 {
			return "", errors.New("invocation path has an invalid segment")
		}
		length, err := strconv.Atoi(remaining[:separator])
		if err != nil || length < 1 {
			return "", errors.New("invocation path has a noncanonical segment")
		}
		stepStart := position + separator + 1
		stepEnd := stepStart + length
		if stepEnd > len(value) {
			return "", errors.New("invocation path has a noncanonical segment")
		}
		if _, err := validateExecutionIdentity("workflow step id", value[stepStart:stepEnd]); err != nil {
			return "", err
		}
		if stepEnd == len(value) {
			return value, nil
		}
		if value[stepEnd] != '/' {
			return "", errors.New("invocation path has a noncanonical segment")
		}
		position = stepEnd + 1
	}
	return "", errors.New("invocation path has an invalid segment")
}
