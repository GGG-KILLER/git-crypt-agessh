package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"github.com/Mic92/ssh-to-age"
	"github.com/alecthomas/kong"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	attrs "github.com/go-git/go-git/v5/plumbing/format/gitattributes"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// NOTE: the Path argument for smudge and clean are just for additional
// information, the file might not actually exist. the actual text gets passed
// is provided via stdin

var CLI struct {
	Init struct {
		Verbose bool `help:"Install filters with verbose flags" short:"v"`
	} `cmd:"" help:"Install git-crypt-agessh configuration in the current repository"`
	DeInit struct {
	} `cmd:"" help:"Remove git-crypt-agessh configuration from the current repository"`
	Smudge struct {
		Path    string `arg:"" name:"path" help:"Path to smudge" type:"path"`
		Verbose bool   `help:"Output additional info to stderr" short:"v"`
	} `cmd:"" help:"Smudge (decrypt) files" hidden:""`
	Clean struct {
		Path    string `arg:"" name:"path" help:"Path to clean" type:"path"`
		Verbose bool   `help:"Output additional info to stderr" short:"v"`
	} `cmd:"" help:"Clean (encrypt) files" hidden:""`
	Textconv struct {
		Path string `arg:"" name:"path" help:"Path to convert" type:"path"`
	} `cmd:"" help:"Convert (decrypt) encrypt into a readable format" hidden:""`
}

func main() {
	ctx := kong.Parse(&CLI)
	switch ctx.Command() {
	case "init":
		cfg, cfgPath := openCfg()

		// if either of the sections are already present, something sus is going on
		if (cfg.Raw.HasSection("filter") && cfg.Raw.Section("filter").HasSubsection("git-crypt-agessh")) ||
			(cfg.Raw.HasSection("diff") && cfg.Raw.Section("diff").HasSubsection("git-crypt-agessh")) {
			log.Fatalf("repository is already (at least partially) initialized, check %s\n", cfgPath)
		}

		// add the options to the config
		cfg.Raw.AddOption("filter", "git-crypt-agessh", "required", "true")
		if CLI.Init.Verbose {
			cfg.Raw.AddOption("filter", "git-crypt-agessh", "smudge", "git-crypt-agessh smudge -v %f")
			cfg.Raw.AddOption("filter", "git-crypt-agessh", "clean", "git-crypt-agessh clean -v %f")
		} else {
			cfg.Raw.AddOption("filter", "git-crypt-agessh", "smudge", "git-crypt-agessh smudge %f")
			cfg.Raw.AddOption("filter", "git-crypt-agessh", "clean", "git-crypt-agessh clean %f")
		}
		cfg.Raw.AddOption("diff", "git-crypt-agessh", "textconv", "git-crypt-agessh textconv")

		saveCfg(cfg, cfgPath)
	case "de-init":
		cfg, cfgPath := openCfg()

		// if either of the sections is missing, that's also strange
		if !((cfg.Raw.HasSection("filter") && cfg.Raw.Section("filter").HasSubsection("git-crypt-agessh")) ||
			(cfg.Raw.HasSection("diff") && cfg.Raw.Section("diff").HasSubsection("git-crypt-agessh"))) {
			log.Fatalln("repository is not initialized")
		}

		// remove the subsectionsn from the config
		cfg.Raw.RemoveSubsection("filter", "git-crypt-agessh")
		cfg.Raw.RemoveSubsection("diff", "git-crypt-agessh")

		saveCfg(cfg, cfgPath)
	case "clean <path>":
		// find the necessary paths
		gitRoot := findGitRoot()
		relPath, err := filepath.Rel(gitRoot, CLI.Clean.Path)
		if err != nil {
			log.Fatalln(err)
		}

		// open the current repository with a memory-backed worktree
		fs := memfs.New()
		repo, err := git.Open(filesystem.NewStorage(osfs.New(filepath.Join(gitRoot, ".git")),
			cache.NewObjectLRUDefault()), fs)
		if err != nil {
			log.Fatalln(err)
		}

		// if we can't find the head, this means there hasn't been any commits yet,
		// so we don't have to compare
		headless := false
		head, err := repo.Head()
		if err != nil {
			if err == plumbing.ErrReferenceNotFound {
				headless = true
			} else {
				log.Fatalln(err)
			}
		}

		if !headless {
			wt, err := repo.Worktree()
			if err != nil {
				log.Fatalln(err)
			}

			// reset the memory worktree to the latest commit
			err = wt.Reset(&git.ResetOptions{Commit: head.Hash(), Mode: git.HardReset})
			if err != nil {
				log.Fatalln(err)
			}

			fileExists := true

			// try to open the filepath that we're examining
			f, err := fs.Open(relPath)
			if err != nil {
				if err == os.ErrNotExist {
					fileExists = false
				} else {
					log.Fatalln(err)
				}
			}

			if fileExists {
				id := getIdentity()

				// decrypt the version of the file in the last commit and write it to a
				// buffer
				r, err := age.Decrypt(f, id)
				if err != nil {
					log.Fatalln(err)
				}

				headBuf := &bytes.Buffer{}
				_, err = io.Copy(headBuf, r)
				if err != nil {
					log.Fatalln(err)
				}

				// also copy the stdin to a buffer
				stdinBuf := &bytes.Buffer{}
				_, err = io.Copy(stdinBuf, os.Stdin)
				if err != nil {
					log.Fatalln(err)
				}

				var noChanges bool

				// if the decrypted versions are identical, since age encryption is
				// non-deterministic...
				if len(headBuf.Bytes()) != len(stdinBuf.Bytes()) {
					noChanges = false
				} else {
					noChanges = true
					for i, b := range headBuf.Bytes() {
						if b != stdinBuf.Bytes()[i] {
							noChanges = false
							break
						}
					}
				}

				if CLI.Clean.Verbose {
					fmt.Fprintf(os.Stderr, "git-crypt-agessh: no changes found while cleaning %s\n", relPath)
				}

				// ...we just reuse the old version
				if noChanges {
					_, err = f.Seek(0, 0)
					if err != nil {
						log.Fatalln(err)
					}

					_, err = io.Copy(os.Stdout, f)
					if err != nil {
						log.Fatalln(err)
					}

					return
				}
			}
		}

		if CLI.Clean.Verbose {
			fmt.Fprintf(os.Stderr, "git-crypt-agessh: found changes while cleaning %s\n", relPath)
		}

		// if we're going to have to encrypt it, we need to look for .gitattributes
		// files to find the corresponding recipient public keys
		pathComponents := []string{string([]rune{filepath.Separator})}
		curr := relPath
		if err != nil {
			log.Fatalln(err)
		}

		for {
			curr = strings.Trim(curr, string([]rune{filepath.Separator}))
			idx := strings.IndexRune(curr, filepath.Separator)
			if idx == -1 {
				pathComponents = append(pathComponents, curr)
				break
			} else {
				pathComponents = append(pathComponents, curr[:idx])
				curr = curr[idx+1:]
			}
		}

		curr, _ = filepath.Split(relPath)
		var recipients []age.Recipient
		var preceedingComment string

		for i := range pathComponents[:len(pathComponents)-1] {
			domain := pathComponents[:i+1]
			if f, err := os.Open(path.Join(path.Join(domain[1:]...), ".gitattributes")); err == nil {
				scanner := bufio.NewScanner(f)
				for scanner.Scan() {
					// if the line is a comment, save it and continue
					if len(scanner.Text()) > 0 && []rune(scanner.Text())[0] == '#' {
						preceedingComment = scanner.Text()
						continue
					}

					// if there was no comment before this line, we don't need to check it
					// because it won't be of use regardless of whether the pattern matches
					if preceedingComment == "" {
						continue
					}

					lineAttrs, err := attrs.ParseAttributesLine(scanner.Text(), domain, true)
					if err != nil {
						log.Fatalln(err)
					}

					// if the target path matches the current pattern...
					if lineAttrs.Pattern != nil && lineAttrs.Pattern.Match(pathComponents) {
						// trim the preceeding comment, split it by commas
						idStrs := strings.Split(strings.TrimSpace(preceedingComment[1:]), ",")

						// then parse all the recipients and add them
						newRecipients := make([]age.Recipient, len(idStrs))
						for i, str := range idStrs {
							newRecipients[i], err = age.ParseX25519Recipient(str)
							if err != nil {
								log.Fatalln(err)
							}
						}
						recipients = append(recipients, newRecipients...)
					}
				}

				// reset preceedingComment to "" because we don't want things to transfer
				// between files
				preceedingComment = ""
			}
		}

		// encrypt stdin and write it to stdout
		w, err := age.Encrypt(os.Stdout, recipients...)
		if err != nil {
			log.Fatalln(err)
		}
		defer func() {
			if err := w.Close(); err != nil {
				log.Fatalln(err)
			}
		}()

		_, err = io.Copy(w, os.Stdin)
		if err != nil {
			log.Fatalln(err)
		}
	case "smudge <path>":
		if CLI.Smudge.Verbose {
			fmt.Fprintf(os.Stderr, "git-crypt-agessh: smudging %s\n", CLI.Smudge.Path)
		}

		id := getIdentity()

		// decrypt stdin and send it to stdout
		r, err := age.Decrypt(os.Stdin, id)
		if err != nil {
			log.Fatalln(err)
		}

		_, err = io.Copy(os.Stdout, r)
		if err != nil {
			log.Fatalln(err)
		}
	case "textconv <path>":
		f, err := os.Open(CLI.Textconv.Path)
		if err != nil {
			log.Fatalln(err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				log.Fatalln(err)
			}
		}()

		// just copy the file input to stdout, behaving like cat, we don't just
		// hardcode the existing cat command because it might not exist on some
		// platforms
		_, err = io.Copy(os.Stdout, f)
		if err != nil {
			log.Fatalln(err)
		}
	default:
		panic(ctx.Command())
	}
}

func findGitRoot() string {
	// start at the current directory
	curr, err := filepath.Abs(".")
	if err != nil {
		log.Fatalln(err)
	}

	for {
		curr = strings.TrimRight(curr, string([]rune{filepath.Separator}))

		if _, err := os.Stat(path.Join(curr, ".git")); err == nil {
			return curr
		}

		if curr == string([]rune{filepath.Separator}) || curr == "" {
			log.Fatalln("no git repository found")
		}

		curr, _ = filepath.Split(curr)
	}
}

func openCfg() (*config.Config, string) {
	cfgPath := path.Join(findGitRoot(), ".git", "config")

	// NOTE: we can't make things more efficient by only opening the file once
	// because we must write with the O_TRUNC flag, but can't read it with that
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		log.Fatalln(err)
	}

	cfg := config.NewConfig()
	err = cfg.Unmarshal(b)
	if err != nil {
		log.Fatalln(err)
	}

	return cfg, cfgPath
}

func saveCfg(cfg *config.Config, cfgPath string) {
	cfgFile, err := os.OpenFile(cfgPath, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		log.Fatalln(err)
	}
	defer func() {
		if err := cfgFile.Close(); err != nil {
			log.Fatalln(err)
		}
	}()

	b, err := cfg.Marshal()
	if err != nil {
		log.Fatalln(err)
	}

	_, err = cfgFile.Write(b)
	if err != nil {
		log.Fatalln(err)
	}
}

func getIdentity() *age.X25519Identity {
	idBytes, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".ssh", "id_ed25519"))
	if err != nil {
		log.Fatalln(err)
	}

	privKeyStr, _, err := agessh.SSHPrivateKeyToAge(idBytes)
	if err != nil {
		log.Fatalln(err)
	}

	id, err := age.ParseX25519Identity(*privKeyStr)
	if err != nil {
		log.Fatalln(err)
	}

	return id
}
