package zed

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/brimdata/zed/zio/anyio"
	"github.com/brimdata/zed/zio/parquetio"
	"github.com/brimdata/zed/zson"
	"github.com/brimdata/zed/ztest"
	"github.com/stretchr/testify/require"
)

func TestZed(t *testing.T) {
	t.Parallel()
	dirs, err := findZTests()
	require.NoError(t, err)
	for d := range dirs {
		d := d
		t.Run(filepath.ToSlash(d), func(t *testing.T) {
			t.Parallel()
			ztest.Run(t, d)
		})
	}
	t.Run("ParquetBoomerang", func(t *testing.T) {
		runParquetBoomerangs(t, dirs)
	})
	t.Run("ZsonBoomerang", func(t *testing.T) {
		runZsonBoomerangs(t, dirs)
	})
}

func findZTests() (map[string]struct{}, error) {
	dirs := map[string]struct{}{}
	pattern := fmt.Sprintf(`.*ztests\%c.*\.yaml$`, filepath.Separator)
	re := regexp.MustCompile(pattern)
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && strings.HasSuffix(path, ".yaml") && re.MatchString(path) {
			dirs[filepath.Dir(path)] = struct{}{}
		}
		return err
	})
	return dirs, err
}

func runZsonBoomerangs(t *testing.T, dirs map[string]struct{}) {
	if testing.Short() {
		return
	}
	const script = `
exec 2>&1
zq -f zson - > baseline.zson &&
zq -i zson -f zson baseline.zson > boomerang.zson &&
diff baseline.zson boomerang.zson
`
	bundles, err := findInputs(t, dirs, script, isValidForZson)
	if err != nil {
		t.Fatal(err)
	}
	shellPath := ztest.ShellPath()
	for _, b := range bundles {
		b := b
		t.Run(b.TestName, func(t *testing.T) {
			t.Parallel()
			err := b.RunScript(shellPath, ".")
			if err != nil {
				err = &BoomerangError{
					*b.Test.Inputs[0].Data,
					b.FileName,
					err,
				}
			}
			require.NoError(t, err)
		})
	}
}

type BoomerangError struct {
	Zson     string
	FileName string
	Err      error
}

func (b *BoomerangError) Error() string {
	return fmt.Sprintf("%s\n=== with this input ===\n\n%s\n\n=== from file ===\n\n%s\n\n", b.Err, b.Zson, b.FileName)
}

func boomerang(script, input string) *ztest.ZTest {
	var empty string
	return &ztest.ZTest{
		Script: script,
		Inputs: []ztest.File{
			{
				Name: "stdin",
				Data: &input,
			},
		},
		Outputs: []ztest.File{
			{
				Name: "stdout",
				Data: &empty,
			},
			{
				Name: "stderr",
				Data: &empty,
			},
		},
	}
}

func expectFailure(b ztest.Bundle) bool {
	if b.Test.ErrorRE != "" {
		return true
	}
	for _, f := range b.Test.Outputs {
		if f.Name == "stderr" {
			return true
		}
	}
	return false
}

func isValidForZson(input string) bool {
	r, err := anyio.NewReader(strings.NewReader(input), zson.NewContext())
	if err != nil {
		return false
	}
	for {
		rec, err := r.Read()
		if err != nil {
			return false
		}
		if rec == nil {
			return true
		}
	}
}

func runParquetBoomerangs(t *testing.T, dirs map[string]struct{}) {
	if testing.Short() {
		return
	}
	const script = `
exec 2>&1
zq -f parquet -o baseline.parquet fuse - &&
zq -i parquet -f parquet -o boomerang.parquet baseline.parquet &&
diff baseline.parquet boomerang.parquet
`
	bundles, err := findInputs(t, dirs, script, isValidForParquet)
	if err != nil {
		t.Fatal(err)
	}
	shellPath := ztest.ShellPath()
	for _, b := range bundles {
		b := b
		t.Run(b.TestName, func(t *testing.T) {
			t.Parallel()
			err := b.RunScript(shellPath, ".")
			if err != nil {
				if s := err.Error(); strings.Contains(s, parquetio.ErrEmptyRecordType.Error()) ||
					strings.Contains(s, parquetio.ErrNullType.Error()) ||
					strings.Contains(s, parquetio.ErrUnionType.Error()) {
					t.Skip("skipping because the Parquet writer cannot handle an input type")
				}
				err = &BoomerangError{
					*b.Test.Inputs[0].Data,
					b.FileName,
					err,
				}
			}
			require.NoError(t, err)
		})
	}
}

func isValidForParquet(input string) bool {
	r, err := anyio.NewReader(strings.NewReader(input), zson.NewContext())
	if err != nil {
		return false
	}
	var found bool
	for {
		rec, err := r.Read()
		if err != nil {
			return false
		}
		if rec == nil {
			return found
		}
		found = true
	}
}

func findInputs(t *testing.T, dirs map[string]struct{}, script string, isValidInput func(string) bool) ([]ztest.Bundle, error) {
	var out []ztest.Bundle
	for path := range dirs {
		bundles, err := ztest.Load(path)
		if err != nil {
			t.Log(err)
			continue
		}
		// Transform the bundles into boomerang tests by taking each
		// source and creating a new ztest.Bundle.
		for _, bundle := range bundles {
			if bundle.Error != nil || expectFailure(bundle) {
				continue
			}
			// Normalize the diffrent kinds of test inputs into
			// a single pattern.
			if input := bundle.Test.Input; isValidInput(input) {
				out = append(out, ztest.Bundle{
					TestName: bundle.TestName,
					FileName: bundle.FileName,
					Test:     boomerang(script, input),
				})
			}
			for _, input := range bundle.Test.Inputs {
				if input.Data != nil && isValidInput(*input.Data) {
					out = append(out, ztest.Bundle{
						TestName: bundle.TestName,
						FileName: bundle.FileName,
						Test:     boomerang(script, *input.Data),
					})
				}
			}
		}
	}
	return out, nil
}
