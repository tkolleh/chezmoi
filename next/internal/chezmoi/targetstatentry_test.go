package chezmoi

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twpayne/go-vfs/vfst"
)

func TestTargetStateEntryApplyAndEqual(t *testing.T) {
	for _, tc1 := range []struct {
		name             string
		targetStateEntry TargetStateEntry
	}{
		{
			name:             "absent",
			targetStateEntry: &TargetStateAbsent{},
		},
		{
			name: "dir",
			targetStateEntry: &TargetStateDir{
				perm: 0o777,
			},
		},
		{
			name: "file",
			targetStateEntry: &TargetStateFile{
				perm: 0o666,
				lazyContents: &lazyContents{
					contents: []byte("bar"),
				},
			},
		},
		{
			name: "file_empty",
			targetStateEntry: &TargetStateFile{
				perm: 0o666,
			},
		},
		{
			name: "file_empty_ok",
			targetStateEntry: &TargetStateFile{
				perm: 0o666,
			},
		},
		{
			name: "file_executable",
			targetStateEntry: &TargetStateFile{
				perm: 0o777,
				lazyContents: &lazyContents{
					contents: []byte("#!/bin/sh\n"),
				},
			},
		},
		{
			name: "present",
			targetStateEntry: &TargetStatePresent{
				perm: 0o666,
			},
		},
		{
			name: "symlink",
			targetStateEntry: &TargetStateSymlink{
				lazyLinkname: &lazyLinkname{
					linkname: "bar",
				},
			},
		},
	} {
		t.Run(tc1.name, func(t *testing.T) {
			for _, tc2 := range []struct {
				name string
				root interface{}
			}{
				{
					name: "not_present",
					root: map[string]interface{}{
						"/home/user": &vfst.Dir{Perm: 0o777},
					},
				},
				{
					name: "existing_dir",
					root: map[string]interface{}{
						"/home/user/foo": &vfst.Dir{Perm: 0o777},
					},
				},
				{
					name: "existing_dir_chmod",
					root: map[string]interface{}{
						"/home/user/foo": &vfst.Dir{Perm: 0o666},
					},
				},
				{
					name: "existing_file_empty",
					root: map[string]interface{}{
						"/home/user/foo": "",
					},
				},
				{
					name: "existing_file_contents",
					root: map[string]interface{}{
						"/home/user/foo": "baz",
					},
				},
				{
					name: "existing_file_chmod",
					root: map[string]interface{}{
						"/home/user/foo": &vfst.File{
							Perm: 0o777,
						},
					},
				},
				{
					name: "existing_symlink",
					root: map[string]interface{}{
						"/home/user/bar": "",
						"/home/user/foo": &vfst.Symlink{Target: "bar"},
					},
				},
				{
					name: "existing_symlink_broken",
					root: map[string]interface{}{
						"/home/user/foo": &vfst.Symlink{Target: "bar"},
					},
				},
			} {
				t.Run(tc2.name, func(t *testing.T) {
					fs, cleanup, err := vfst.NewTestFS(tc2.root)
					require.NoError(t, err)
					defer cleanup()
					s := newTestRealSystem(fs)

					// Read the initial destination state entry from fs.
					destStateEntry, err := NewDestStateEntry(s, "/home/user/foo")
					require.NoError(t, err)

					// Apply the target state entry.
					targetSystem := newTestRealSystem(fs)
					require.NoError(t, tc1.targetStateEntry.Apply(targetSystem, destStateEntry, Umask))

					// Verify that the destination state entry matches the
					// desired state.
					vfst.RunTests(t, fs, "", vfst.TestPath("/home/user/foo", targetStateTest(t, tc1.targetStateEntry)...))

					// Read the updated destination state entry from fs and
					// verify that it is equal to the target state entry.
					newDestStateEntry, err := NewDestStateEntry(s, "/home/user/foo")
					require.NoError(t, err)
					equal, err := tc1.targetStateEntry.Equal(newDestStateEntry, Umask)
					require.NoError(t, err)
					require.True(t, equal)
				})
			}
		})
	}
}

func TestTargetStateRename(t *testing.T) {
	fs, cleanup, err := vfst.NewTestFS(map[string]interface{}{
		"/home/user/.local/share/chezmoi/dot_bashrc": "# contents of .bashrc\n",
	})
	require.NoError(t, err)
	defer cleanup()
	s := newTestRealSystem(fs)

	destStateEntry, err := NewDestStateEntry(s, "/home/user/.local/share/chezmoi/dot_bashrc")
	require.NoError(t, err)

	targetStateRename := &TargetStateRename{
		newName: "/home/user/.local/share/chezmoi/dot_bashrc.tmpl",
	}
	require.NoError(t, targetStateRename.Apply(s, destStateEntry, Umask))

	vfst.RunTests(t, fs, "",
		vfst.TestPath("/home/user/.local/share/chezmoi/dot_bashrc",
			vfst.TestDoesNotExist,
		),
		vfst.TestPath("/home/user/.local/share/chezmoi/dot_bashrc.tmpl",
			vfst.TestContentsString("# contents of .bashrc\n"),
		),
	)
}

func targetStateTest(t *testing.T, ts TargetStateEntry) []vfst.PathTest {
	switch ts := ts.(type) {
	case *TargetStateAbsent:
		return []vfst.PathTest{
			vfst.TestDoesNotExist,
		}
	case *TargetStateDir:
		return []vfst.PathTest{
			vfst.TestIsDir,
			vfst.TestModePerm(ts.perm &^ Umask),
		}
	case *TargetStateFile:
		expectedContents, err := ts.Contents()
		require.NoError(t, err)
		return []vfst.PathTest{
			vfst.TestModeIsRegular,
			vfst.TestContents(expectedContents),
			vfst.TestModePerm(ts.perm &^ Umask),
		}
	case *TargetStatePresent:
		return []vfst.PathTest{
			vfst.TestModeIsRegular,
			vfst.TestModePerm(ts.perm &^ Umask),
		}
	case *TargetStateRename:
		// FIXME test for presence of newName
		return []vfst.PathTest{
			vfst.TestDoesNotExist,
		}
	case *TargetStateScript:
		return nil // FIXME how to verify scripts?
	case *TargetStateSymlink:
		expectedLinkname, err := ts.Linkname()
		require.NoError(t, err)
		return []vfst.PathTest{
			vfst.TestModeType(os.ModeSymlink),
			vfst.TestSymlinkTarget(expectedLinkname),
		}
	default:
		return nil
	}
}
