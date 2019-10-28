package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"path"

	nc "github.com/rthornton128/goncurses"
)

const (
	title        = "Ubuntu Core for YoyoDyne Retro Encabulator 550"
	goBackOption = "Go Back"
)

var (
	PrevMenuError = fmt.Errorf("Return to previous menu")
)

type Chooser struct {
	scr *nc.Window
}

func (c *Chooser) Init() error {
	var err error
	c.scr, err = nc.Init()
	if err != nil {
		return err
	}

	nc.Raw(true)
	nc.Echo(false)
	nc.Cursor(0)
	c.scr.Timeout(0)
	c.scr.Clear()
	c.scr.Keypad(true)

	return nil
}

func (c *Chooser) Deinit() {
	nc.End()
}

type MenuOption struct {
	text    string
	handler func(*Chooser, ...interface{}) error
}

func (c *Chooser) DisplayMenu(title, prompt string, options []MenuOption, firstOption int) (int, error) {
	c.scr.Clear()
	c.scr.Printf("%s\n\n%s\n", title, prompt)

	items := make([]*nc.MenuItem, len(options))

	for i, option := range options {
		items[i], _ = nc.NewItem(fmt.Sprintf("%2d. %s", i+firstOption, option.text), "")
		defer items[i].Free()
	}

	menu, err := nc.NewMenu(items)
	if err != nil {
		return 0, err
	}
	defer menu.Free()

	win, err := nc.NewWindow(20, 40, 3, 0)
	if err != nil {
		return 0, err
	}
	win.Keypad(true)

	menu.SetWindow(win)
	menu.SubWindow(win.Derived(18, 38, 1, 1))
	menu.Mark("> ")

	c.scr.Refresh()

	menu.Post()
	defer menu.UnPost()
	win.Refresh()

	for {
		ch := win.GetChar()
		if int(ch) >= 0+firstOption && int(ch) <= 9 {
			return int(ch) - firstOption, nil
		}

		key := nc.KeyString(ch)
		switch key {
		case "enter":
			return menu.Current(nil).Index(), nil
		case "down":
			menu.Driver(nc.REQ_DOWN)
		case "up":
			menu.Driver(nc.REQ_UP)
		}

		win.Refresh()
	}
}

//

func main() {
	output := flag.String("output", "/run/chooser.out", "Output file location")
	seed := flag.String("seed", "/run/ubuntu-seed", "Ubuntu-seed location")
	flag.Parse()

	c := &Chooser{}
	c.Init()
	defer c.Deinit()

	mainMenu := []MenuOption{
		{"Start normally", runHandler},
		{"Start into a previous version  >", chooseHandler},
		{"Recover                        >", recoverHandler},
		{"Reinstall                      >", reinstallHandler},
	}

	for {
		opt, err := c.DisplayMenu(title, "Use arrow keys then Enter:", mainMenu, 1)
		if err != nil {
			c.Deinit()
			log.Fatalf("internal error: %s", err)
		}
		if err := mainMenu[opt].handler(c, *seed, *output); err != PrevMenuError {
			if err != nil {
				c.Deinit()
				log.Fatalf("internal error: %s", err)
			}
			break
		}
	}
}

func runHandler(c *Chooser, parms ...interface{}) error {
	// do nothing
	return nil
}

func chooseHandler(c *Chooser, parms ...interface{}) error {
	seed := parms[0].(string)
	version, err := chooseSystem(c, "Start into a previous version:", seed)
	if err != nil {
		return err
	}
	if version == "" {
		return PrevMenuError
	}

	// do something with chosen version
	_ = version

	return nil
}

func recoverHandler(c *Chooser, parms ...interface{}) error {
	// set system to boot in recover mode
	return nil
}

func reinstallHandler(c *Chooser, parms ...interface{}) error {
	// reinstall system
	return nil
}

func chooseSystem(c *Chooser, prompt, seedDir string) (string, error) {
	versions, err := getRecoveryVersions(seedDir)
	if err != nil {
		return "", fmt.Errorf("cannot get recovery versions: %s", err)
	}

	versionOptions := make([]MenuOption, len(versions)+1)
	versionOptions[0] = MenuOption{goBackOption, nil}
	for i, ver := range versions {
		versionOptions[i+1] = MenuOption{ver, nil}
	}

	index, err := c.DisplayMenu(title, prompt, versionOptions, 0)
	if err != nil {
		return "", fmt.Errorf("cannot display menu: %s", err)
	}
	if index == 0 { // "Go Back" selected
		return "", nil
	}
	version := versionOptions[index].text

	return version, nil
}

func getRecoveryVersions(mnt string) ([]string, error) {
	files, err := ioutil.ReadDir(path.Join(mnt, "systems"))
	if err != nil {
		return []string{}, fmt.Errorf("cannot read recovery list: %s", err)
	}
	list := make([]string, len(files))
	for i, f := range files {
		list[i] = f.Name()
	}
	return list, nil
}
