package admin

import (
	"reflect"
	"strings"
	"testing"

	"github.com/synqronlabs/mxraven-go/admin/api"
)

func TestFacadeCoversEveryGeneratedOperation(t *testing.T) {
	clientType := reflect.TypeOf((*api.Client)(nil))
	workspaceType := reflect.TypeOf(WorkspaceServices{})

	for i := 0; i < clientType.NumMethod(); i++ {
		operation := clientType.Method(i).Name
		target, ok := facadeOperations[operation]
		if !ok {
			t.Errorf("generated operation %s has no facade method", operation)
			continue
		}
		fieldName, methodName, ok := strings.Cut(target, ".")
		if !ok {
			t.Errorf("operation %s has malformed facade target %q", operation, target)
			continue
		}
		field, ok := workspaceType.FieldByName(fieldName)
		if !ok {
			t.Errorf("operation %s targets missing workspace service %s", operation, fieldName)
			continue
		}
		if _, ok := field.Type.MethodByName(methodName); !ok {
			t.Errorf("operation %s targets missing method %s.%s", operation, fieldName, methodName)
		}
	}

	for operation := range facadeOperations {
		if _, ok := clientType.MethodByName(operation); !ok {
			t.Errorf("facade manifest contains unknown operation %s", operation)
		}
	}
}
