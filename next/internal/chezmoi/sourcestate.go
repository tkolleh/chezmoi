package chezmoi

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/coreos/go-semver/semver"
	vfs "github.com/twpayne/go-vfs"
	"go.uber.org/multierr"
)

// A SourceState is a source state.
type SourceState struct {
	entries              map[string]SourceStateEntry
	system               System
	sourceDir            string
	destDir              string
	umask                os.FileMode
	encryptionTool       EncryptionTool
	ignore               *PatternSet
	minVersion           semver.Version
	priorityTemplateData map[string]interface{}
	templateData         map[string]interface{}
	templateFuncs        template.FuncMap
	templateOptions      []string
	templates            map[string]*template.Template
}

// A SourceStateOption sets an option on a source state.
type SourceStateOption func(*SourceState)

// WithDestDir sets the destination directory.
func WithDestDir(destDir string) SourceStateOption {
	return func(s *SourceState) {
		s.destDir = destDir
	}
}

// WithEncryptionTool set the encryption tool.
func WithEncryptionTool(encryptionTool EncryptionTool) SourceStateOption {
	return func(s *SourceState) {
		s.encryptionTool = encryptionTool
	}
}

// WithPriorityTemplateData adds priority template data.
func WithPriorityTemplateData(priorityTemplateData map[string]interface{}) SourceStateOption {
	return func(s *SourceState) {
		recursiveMerge(s.priorityTemplateData, priorityTemplateData)
		recursiveMerge(s.templateData, s.priorityTemplateData)
	}
}

// WithSourceDir sets the source directory.
func WithSourceDir(sourceDir string) SourceStateOption {
	return func(s *SourceState) {
		s.sourceDir = sourceDir
	}
}

// WithSystem sets the system.
func WithSystem(system System) SourceStateOption {
	return func(s *SourceState) {
		s.system = system
	}
}

// WithTemplateData adds template data.
func WithTemplateData(templateData map[string]interface{}) SourceStateOption {
	return func(s *SourceState) {
		recursiveMerge(s.templateData, templateData)
		recursiveMerge(s.templateData, s.priorityTemplateData)
	}
}

// WithTemplateFuncs sets the template functions.
func WithTemplateFuncs(templateFuncs template.FuncMap) SourceStateOption {
	return func(s *SourceState) {
		s.templateFuncs = templateFuncs
	}
}

// WithTemplateOptions sets the template options.
func WithTemplateOptions(templateOptions []string) SourceStateOption {
	return func(s *SourceState) {
		s.templateOptions = templateOptions
	}
}

// WithUmask sets the umask.
func WithUmask(umask os.FileMode) SourceStateOption {
	return func(s *SourceState) {
		s.umask = umask
	}
}

// NewSourceState creates a new source state with the given options.
func NewSourceState(options ...SourceStateOption) *SourceState {
	s := &SourceState{
		entries:              make(map[string]SourceStateEntry),
		umask:                Umask,
		encryptionTool:       &nullEncryptionTool{},
		ignore:               NewPatternSet(),
		priorityTemplateData: make(map[string]interface{}),
		templateData:         make(map[string]interface{}),
		templateOptions:      DefaultTemplateOptions,
	}
	for _, option := range options {
		option(s)
	}
	return s
}

// AddOptions are options to SourceState.Add.
type AddOptions struct {
	AutoTemplate bool
	Empty        bool
	Encrypt      bool
	Exact        bool
	Include      *IncludeSet
	Template     bool
	umask        os.FileMode
}

// Add adds sourceStateEntry to s.
func (s *SourceState) Add(sourceSystem System, destPathInfos map[string]os.FileInfo, options *AddOptions) error {
	destPaths := make([]string, 0, len(destPathInfos))
	for destPath := range destPathInfos {
		destPaths = append(destPaths, destPath)
	}
	sort.Strings(destPaths)
	targetSourceState := &SourceState{
		entries: make(map[string]SourceStateEntry),
	}
	for _, destPath := range destPaths {
		// FIXME rename/remove old
		// targetName := strings.TrimPrefix(destPath, s.destDir+"/") // remove?
		parentDir := "" // FIXME
		sourceStateEntry, err := s.sourceStateEntry(sourceSystem, destPath, destPathInfos[destPath], parentDir, options)
		if err != nil {
			return err
		}
		if sourceStateEntry != nil {
			targetSourceState.entries[sourceStateEntry.Path()] = sourceStateEntry
		}
	}
	return targetSourceState.ApplyAll(sourceSystem, s.sourceDir, options.Include, options.umask)
}

// ApplyAll updates targetDir in fs to match s.
func (s *SourceState) ApplyAll(targetSystem System, targetDir string, include *IncludeSet, umask os.FileMode) error {
	for _, targetName := range s.sortedTargetNames() {
		if err := s.ApplyOne(targetSystem, targetDir, targetName, include, umask); err != nil {
			return err
		}
	}
	return nil
}

// ApplyOne updates targetName in targetDir on fs to match s using s.
func (s *SourceState) ApplyOne(targetSystem System, targetDir, targetName string, include *IncludeSet, umask os.FileMode) error {
	targetStateEntry, err := s.entries[targetName].TargetStateEntry()
	if err != nil {
		return err
	}

	if !include.IncludeTargetStateEntry(targetStateEntry) {
		return nil
	}

	targetPath := path.Join(targetDir, targetName)
	destStateEntry, err := NewDestStateEntry(targetSystem, targetPath)
	if err != nil {
		return err
	}

	return targetStateEntry.Apply(targetSystem, destStateEntry, umask)
}

// Entries returns s's source state entries.
func (s *SourceState) Entries() map[string]SourceStateEntry {
	return s.entries
}

// Ignored returns if targetName is ignored.
func (s *SourceState) Ignored(targetName string) bool {
	return s.ignore.Match(targetName)
}

// TargetNames returns all of s's target names in alphabetical order.
func (s *SourceState) TargetNames() []string {
	targetNames := make([]string, 0, len(s.entries))
	for targetName := range s.entries {
		targetNames = append(targetNames, targetName)
	}
	sort.Strings(targetNames)
	return targetNames
}

// Entry returns the source state entry for targetName.
func (s *SourceState) Entry(targetName string) (SourceStateEntry, bool) {
	sourceStateEntry, ok := s.entries[targetName]
	return sourceStateEntry, ok
}

// Evaluate evaluates every target state entry in s.
func (s *SourceState) Evaluate() error {
	for _, targetName := range s.sortedTargetNames() {
		sourceStateEntry := s.entries[targetName]
		if err := sourceStateEntry.Evaluate(); err != nil {
			return err
		}
		targetStateEntry, err := sourceStateEntry.TargetStateEntry()
		if err != nil {
			return err
		}
		if err := targetStateEntry.Evaluate(); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteTemplateData returns the result of executing template data.
func (s *SourceState) ExecuteTemplateData(name string, data []byte) ([]byte, error) {
	tmpl, err := template.New(name).Option(s.templateOptions...).Funcs(s.templateFuncs).Parse(string(data))
	if err != nil {
		return nil, err
	}
	for name, t := range s.templates {
		tmpl, err = tmpl.AddParseTree(name, t.Tree)
		if err != nil {
			return nil, err
		}
	}
	output := &bytes.Buffer{}
	if err = tmpl.ExecuteTemplate(output, name, s.TemplateData()); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// MinVersion returns the minimum version for which s is valid.
func (s *SourceState) MinVersion() semver.Version {
	return s.minVersion
}

// MustEntry returns the source state entry associated with targetName, and
// panics if it does not exist.
func (s *SourceState) MustEntry(targetName string) SourceStateEntry {
	sourceStateEntry, ok := s.entries[targetName]
	if !ok {
		panic(fmt.Sprintf("%s: no source state entry", targetName))
	}
	return sourceStateEntry
}

// Read reads a source state from sourcePath.
func (s *SourceState) Read() error {
	info, err := s.system.Lstat(s.sourceDir)
	switch {
	case os.IsNotExist(err):
		return nil
	case err != nil:
		return err
	case !info.IsDir():
		return fmt.Errorf("%s: not a directory", s.sourceDir)
	}

	// Read all source entries.
	allSourceStateEntries := make(map[string][]SourceStateEntry)
	sourceDirPrefix := s.sourceDir + "/"
	if err := vfs.WalkSlash(s.system, s.sourceDir, func(sourcePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if sourcePath == s.sourceDir {
			return nil
		}
		relPath := strings.TrimPrefix(sourcePath, sourceDirPrefix)
		sourceDirName, sourceName := path.Split(relPath)
		targetDirName := getTargetDirName(sourceDirName)
		// Follow symlinks from the source directory.
		if info.Mode()&os.ModeType == os.ModeSymlink {
			info, err = s.system.Stat(sourcePath)
			if err != nil {
				return err
			}
		}
		switch {
		case strings.HasPrefix(info.Name(), dataName):
			return s.addTemplateData(sourcePath)
		case info.Name() == ignoreName:
			// .chezmoiignore is interpreted as a template. vfs.WalkSlash walks
			// in alphabetical order, so, luckily for us, .chezmoidata will be
			// read before .chezmoiignore, so data in .chezmoidata is available
			// to be used in .chezmoiignore. Unluckily for us, .chezmoitemplates
			// will be read afterwards so partial templates will not be
			// available in .chezmoiignore.
			return s.addPatterns(s.ignore, sourcePath, sourceDirName)
		case info.Name() == removeName:
			// The comment about .chezmoiignore and templates applies to
			// .chezmoiremove too.
			removePatterns := NewPatternSet()
			if err := s.addPatterns(removePatterns, sourcePath, targetDirName); err != nil {
				return err
			}
			matches, err := removePatterns.Glob(s.system.UnderlyingFS(), s.destDir+"/")
			if err != nil {
				return err
			}
			n := 0
			for _, match := range matches {
				if !s.ignore.Match(match) {
					matches[n] = match
					n++
				}
			}
			matches = matches[:n]
			sourceStateEntry := &SourceStateRemove{
				path: sourcePath,
			}
			for _, match := range matches {
				allSourceStateEntries[match] = append(allSourceStateEntries[match], sourceStateEntry)
			}
			return nil
		case info.Name() == templatesDirName:
			if err := s.addTemplatesDir(sourcePath); err != nil {
				return err
			}
			return vfs.SkipDir
		case info.Name() == versionName:
			return s.addVersionFile(sourcePath)
		case strings.HasPrefix(info.Name(), ChezmoiPrefix):
			fallthrough
		case strings.HasPrefix(info.Name(), ignorePrefix):
			if info.IsDir() {
				return vfs.SkipDir
			}
			return nil
		case info.IsDir():
			da := parseDirAttributes(sourceName)
			targetName := path.Join(targetDirName, da.Name)
			if s.ignore.Match(targetName) {
				return nil
			}
			sourceStateEntry := s.newSourceStateDir(sourcePath, da)
			allSourceStateEntries[targetName] = append(allSourceStateEntries[targetName], sourceStateEntry)
			return nil
		case info.Mode().IsRegular():
			fa := parseFileAttributes(sourceName)
			targetName := path.Join(targetDirName, fa.Name)
			if s.ignore.Match(targetName) {
				return nil
			}
			sourceStateEntry := s.newSourceStateFile(sourcePath, fa, targetName)
			allSourceStateEntries[targetName] = append(allSourceStateEntries[targetName], sourceStateEntry)
			return nil
		default:
			return &unsupportedFileTypeError{
				path: sourcePath,
				mode: info.Mode(),
			}
		}
	}); err != nil {
		return err
	}

	// Remove all ignored targets.
	for targetName := range allSourceStateEntries {
		if s.ignore.Match(targetName) {
			delete(allSourceStateEntries, targetName)
		}
	}

	// Generate SourceStateRemoves for exact directories.
	for targetName, sourceStateEntries := range allSourceStateEntries {
		if len(sourceStateEntries) != 1 {
			continue
		}
		sourceStateDir, ok := sourceStateEntries[0].(*SourceStateDir)
		if !ok {
			continue
		}
		if !sourceStateDir.Attributes.Exact {
			continue
		}
		sourceStateRemove := &SourceStateRemove{
			path: sourceStateDir.Path(),
		}
		infos, err := s.system.ReadDir(path.Join(s.destDir, targetName))
		switch {
		case err == nil:
			for _, info := range infos {
				name := info.Name()
				if name == "." || name == ".." {
					continue
				}
				targetEntryName := path.Join(targetName, name)
				if _, ok := allSourceStateEntries[targetEntryName]; ok {
					continue
				}
				if s.ignore.Match(targetEntryName) {
					continue
				}
				allSourceStateEntries[targetEntryName] = append(allSourceStateEntries[targetEntryName], sourceStateRemove)
			}
		case os.IsNotExist(err):
			// Do nothing.
		default:
			return err
		}
	}

	// Check for duplicate source entries with the same target name. Iterate
	// over the target names in order so that any error is deterministic.
	targetNames := make([]string, 0, len(allSourceStateEntries))
	for targetName := range allSourceStateEntries {
		targetNames = append(targetNames, targetName)
	}
	sort.Strings(targetNames)
	for _, targetName := range targetNames {
		sourceStateEntries := allSourceStateEntries[targetName]
		if len(sourceStateEntries) == 1 {
			continue
		}
		sourcePaths := make([]string, 0, len(sourceStateEntries))
		for _, sourceStateEntry := range sourceStateEntries {
			sourcePaths = append(sourcePaths, sourceStateEntry.Path())
		}
		err = multierr.Append(err, &duplicateTargetError{
			targetName:  targetName,
			sourcePaths: sourcePaths,
		})
	}
	if err != nil {
		return err
	}

	// Populate s.Entries with the unique source entry for each target.
	for targetName, sourceEntries := range allSourceStateEntries {
		s.entries[targetName] = sourceEntries[0]
	}

	return nil
}

// TemplateData returns s's template data.
func (s *SourceState) TemplateData() map[string]interface{} {
	return s.templateData
}

func (s *SourceState) addPatterns(patternSet *PatternSet, sourcePath, relPath string) error {
	data, err := s.executeTemplate(sourcePath)
	if err != nil {
		return err
	}
	dir := path.Dir(relPath)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var lineNumber int
	for scanner.Scan() {
		lineNumber++
		text := scanner.Text()
		if index := strings.IndexRune(text, '#'); index != -1 {
			text = text[:index]
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		include := true
		if strings.HasPrefix(text, "!") {
			include = false
			text = strings.TrimPrefix(text, "!")
		}
		pattern := path.Join(dir, text)
		if err := patternSet.Add(pattern, include); err != nil {
			return fmt.Errorf("%s:%d: %w", sourcePath, lineNumber, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%s: %w", sourcePath, err)
	}
	return nil
}

func (s *SourceState) addTemplateData(sourcePath string) error {
	_, name := path.Split(sourcePath)
	suffix := strings.TrimPrefix(name, dataName+".")
	format, ok := Formats[strings.ToLower(suffix)]
	if !ok {
		return fmt.Errorf("%s: unknown format", sourcePath)
	}
	data, err := s.system.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("%s: %w", sourcePath, err)
	}
	var templateData map[string]interface{}
	if err := format.Decode(data, &templateData); err != nil {
		return fmt.Errorf("%s: %w", sourcePath, err)
	}
	recursiveMerge(s.templateData, templateData)
	recursiveMerge(s.templateData, s.priorityTemplateData)
	return nil
}

func (s *SourceState) addTemplatesDir(templateDir string) error {
	templateDirPrefix := templateDir + "/"
	return vfs.WalkSlash(s.system, templateDir, func(templatePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		switch {
		case info.Mode().IsRegular():
			contents, err := s.system.ReadFile(templatePath)
			if err != nil {
				return err
			}
			name := strings.TrimPrefix(templatePath, templateDirPrefix)
			tmpl, err := template.New(name).Option(s.templateOptions...).Funcs(s.templateFuncs).Parse(string(contents))
			if err != nil {
				return err
			}
			if s.templates == nil {
				s.templates = make(map[string]*template.Template)
			}
			s.templates[name] = tmpl
			return nil
		case info.IsDir():
			return nil
		default:
			return &unsupportedFileTypeError{
				path: templatePath,
				mode: info.Mode(),
			}
		}
	})
}

// addVersionFile reads a .chezmoiversion file from source path and updates s's
// minimum version if it contains a more recent version than the current minimum
// version.
func (s *SourceState) addVersionFile(sourcePath string) error {
	data, err := s.system.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	version, err := semver.NewVersion(strings.TrimSpace(string(data)))
	if err != nil {
		return err
	}
	if s.minVersion.LessThan(*version) {
		s.minVersion = *version
	}
	return nil
}

func (s *SourceState) executeTemplate(path string) ([]byte, error) {
	data, err := s.system.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return s.ExecuteTemplateData(path, data)
}

func (s *SourceState) newSourceStateDir(sourcePath string, da DirAttributes) *SourceStateDir {
	targetStateDir := &TargetStateDir{
		perm: da.Perm(),
	}

	return &SourceStateDir{
		path:             sourcePath,
		Attributes:       da,
		targetStateEntry: targetStateDir,
	}
}

func (s *SourceState) newSourceStateFile(sourcePath string, fa FileAttributes, targetName string) *SourceStateFile {
	lazyContents := &lazyContents{
		contentsFunc: func() ([]byte, error) {
			contents, err := s.system.ReadFile(sourcePath)
			if err != nil {
				return nil, err
			}
			if !fa.Encrypted {
				return contents, nil
			}
			// FIXME pass targetName as filenameHint
			return s.encryptionTool.Decrypt(sourcePath, contents)
		},
	}

	var targetStateEntryFunc func() (TargetStateEntry, error)
	switch fa.Type {
	case SourceFileTypeFile:
		targetStateEntryFunc = func() (TargetStateEntry, error) {
			contents, err := lazyContents.Contents()
			if err != nil {
				return nil, err
			}
			if fa.Template {
				contents, err = s.ExecuteTemplateData(sourcePath, contents)
				if err != nil {
					return nil, err
				}
			}
			if !fa.Empty && isEmpty(contents) {
				return &TargetStateAbsent{}, nil
			}
			return &TargetStateFile{
				lazyContents: newLazyContents(contents),
				perm:         fa.Perm(),
			}, nil
		}
	case SourceFileTypePresent:
		targetStateEntryFunc = func() (TargetStateEntry, error) {
			contents, err := lazyContents.Contents()
			if err != nil {
				return nil, err
			}
			if fa.Template {
				contents, err = s.ExecuteTemplateData(sourcePath, contents)
				if err != nil {
					return nil, err
				}
			}
			return &TargetStatePresent{
				lazyContents: newLazyContents(contents),
				perm:         fa.Perm(),
			}, nil
		}
	case SourceFileTypeScript:
		targetStateEntryFunc = func() (TargetStateEntry, error) {
			contents, err := lazyContents.Contents()
			if err != nil {
				return nil, err
			}
			if fa.Template {
				contents, err = s.ExecuteTemplateData(sourcePath, contents)
				if err != nil {
					return nil, err
				}
			}
			return &TargetStateScript{
				lazyContents: newLazyContents(contents),
				name:         targetName,
				once:         fa.Once,
			}, nil
		}
	case SourceFileTypeSymlink:
		targetStateEntryFunc = func() (TargetStateEntry, error) {
			linknameBytes, err := lazyContents.Contents()
			if err != nil {
				return nil, err
			}
			if fa.Template {
				linknameBytes, err = s.ExecuteTemplateData(sourcePath, linknameBytes)
				if err != nil {
					return nil, err
				}
			}
			return &TargetStateSymlink{
				lazyLinkname: newLazyLinkname(string(bytes.TrimSpace(linknameBytes))),
			}, nil
		}
	case SourceFileTypeSymlinked:
		targetStateEntryFunc = func() (TargetStateEntry, error) {
			basepath := filepath.Join(s.targetDir, targetName)
			targpath := sourcePath
			relPath, err := filepath.Rel(basepath, targpath)
			if err != nil {
				return nil, err
			}
			return &TargetStateSymlink{
				lazyLinkname: newLazyLinkname(relPath),
			}, nil
		}
	default:
		panic(fmt.Sprintf("%d: unsupported type", fa.Type))
	}

	return &SourceStateFile{
		lazyContents:         lazyContents,
		path:                 sourcePath,
		Attributes:           fa,
		targetStateEntryFunc: targetStateEntryFunc,
	}
}

// sortedTargetNames returns all of s's target names in order.
func (s *SourceState) sortedTargetNames() []string {
	targetNames := make([]string, 0, len(s.entries))
	for targetName := range s.entries {
		targetNames = append(targetNames, targetName)
	}
	sort.Slice(targetNames, func(i, j int) bool {
		orderI := s.entries[targetNames[i]].Order()
		orderJ := s.entries[targetNames[j]].Order()
		switch {
		case orderI < orderJ:
			return true
		case orderI == orderJ:
			return targetNames[i] < targetNames[j]
		default:
			return false
		}
	})
	return targetNames
}

func (s *SourceState) sourceStateEntry(system System, destPath string, info os.FileInfo, parentDir string, options *AddOptions) (SourceStateEntry, error) {
	destStateEntry, err := NewDestStateEntry(system, destPath)
	if err != nil {
		return nil, err
	}
	if !options.Include.IncludeDestStateEntry(destStateEntry) {
		return nil, nil
	}
	// FIXME create parents
	switch destStateEntry := destStateEntry.(type) {
	case *DestStateAbsent:
		return nil, fmt.Errorf("%s: not found", destPath)
	case *DestStateDir:
		attributes := DirAttributes{
			Name:    info.Name(),
			Exact:   options.Exact,
			Private: UNIXFileModes && info.Mode().Perm()&0o77 == 0,
		}
		return &SourceStateDir{
			path:       path.Join(parentDir, attributes.BaseName()),
			Attributes: attributes,
			targetStateEntry: &TargetStateDir{
				perm: 0o777,
			},
		}, nil
	case *DestStateFile:
		attributes := FileAttributes{
			Name:       info.Name(),
			Type:       SourceFileTypeFile,
			Empty:      options.Empty,
			Encrypted:  options.Encrypt,
			Executable: UNIXFileModes && info.Mode().Perm()&0o111 != 0,
			Private:    UNIXFileModes && info.Mode().Perm()&0o77 == 0,
			Template:   options.Template || options.AutoTemplate,
		}
		contents, err := destStateEntry.Contents()
		if err != nil {
			return nil, err
		}
		if options.AutoTemplate {
			contents = autoTemplate(contents, s.TemplateData())
		}
		if len(contents) == 0 && !options.Empty {
			return nil, nil
		}
		lazyContents := &lazyContents{
			contents: contents,
		}
		return &SourceStateFile{
			path:         path.Join(parentDir, attributes.BaseName()),
			Attributes:   attributes,
			lazyContents: lazyContents,
			targetStateEntry: &TargetStateFile{
				lazyContents: lazyContents,
				perm:         0o666,
			},
		}, nil
	case *DestStateSymlink:
		attributes := FileAttributes{
			Name:     info.Name(),
			Type:     SourceFileTypeSymlink,
			Template: options.Template || options.AutoTemplate,
		}
		linkname, err := destStateEntry.Linkname()
		if err != nil {
			return nil, err
		}
		contents := []byte(linkname)
		if options.AutoTemplate {
			contents = autoTemplate(contents, s.TemplateData())
		}
		lazyContents := &lazyContents{
			contents: contents,
		}
		return &SourceStateFile{
			path:         path.Join(parentDir, attributes.BaseName()),
			Attributes:   attributes,
			lazyContents: lazyContents,
			targetStateEntry: &TargetStateFile{
				lazyContents: lazyContents,
				perm:         0o666,
			},
		}, nil
	default:
		panic(fmt.Sprintf("%T: unsupported type", destStateEntry))
	}
}

// getTargetDirName returns the target directory name of sourceDirName.
func getTargetDirName(sourceDirName string) string {
	sourceNames := strings.Split(sourceDirName, "/")
	targetNames := make([]string, 0, len(sourceNames))
	for _, sourceName := range sourceNames {
		da := parseDirAttributes(sourceName)
		targetNames = append(targetNames, da.Name)
	}
	return strings.Join(targetNames, "/")
}
