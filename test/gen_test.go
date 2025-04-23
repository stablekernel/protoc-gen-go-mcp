package gen_test

import (
	"fmt"
	"google.golang.org/protobuf/compiler/protogen"
	"os"
	"path"
	"path/filepath"
	"protoc-gen-go-mcp/internal"
	"testing"

	"github.com/pseudomuto/protokit/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Scenario struct {
	Name string
}

var scenarios = []Scenario{
	{Name: "mcp"},
}

// TestGen uses data in testdata/ to make requests to generate code and compare it to a snapshot
func TestGen(t *testing.T) {
	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			protoPaths, err := filepath.Glob(fmt.Sprintf("testdata/%s/**.proto", scenario.Name))
			require.NoError(t, err)
			require.NotEmpty(t, protoPaths)

			// There is an assumption a proto file (or set of proto files for a specific test) should only exist
			// one level deep in the directory structure and therefore there should only be one snapshot path per
			// set of proto files to validate against
			var snapshotPaths []string
			for _, p := range protoPaths {
				snapshotPaths = append(snapshotPaths, path.Dir(p))
			}
			require.Len(t, snapshotPaths, 1, "there can only be one! (output path)")

			generateAndCheckResult(t, snapshotPaths[0], protoPaths)
		})
	}
}

func generateAndCheckResult(t *testing.T, snapshotPath string, filesToGen []string) {
	// Don't forget to compile this file if you change protos!
	fds, err := utils.LoadDescriptorSet("image.binpb")
	require.NoError(t, err)

	p, err := protogen.Options{}.New(utils.CreateGenRequest(fds, filesToGen...))
	if err != nil {
		panic(err)
	}

	var generatedFiles []*internal.File
	for _, f := range p.Files {
		generatedFiles = append(generatedFiles, internal.GenerateFile(p, f))
	}

	for _, f := range generatedFiles {
		assert.NotNil(t, f.Name)
		t.Run(path.Base(f.Name), func(t *testing.T) {
			expectedFilePath := path.Join(snapshotPath, f.Name)
			_, statErr := os.Stat(expectedFilePath)
			require.NoError(t, statErr)

			expectedFile, err := os.ReadFile(expectedFilePath)
			require.NoError(t, err)

			content, err := f.Content()
			require.NoError(t, err)

			assert.Equal(t, string(expectedFile), string(content))
		})
	}
}
