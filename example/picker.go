package example

import "github.com/amterp/radish"

// prompter is the seam that makes an interactive flow testable: production
// passes radish's terminal driver, the snapshot suite passes a scripted one.
// The prompt Model itself is identical either way, so the test exercises the
// real rendering rather than a stand-in for it.
type prompter interface {
	Run(radish.Model) (radish.Result, radish.Model, error)
}

// pickFood is the toy interactive flow under test.
func pickFood(p prompter, options []string) (string, error) {
	m := radish.NewSelect().Title("Pick a food").Options(options...).Height(3)
	res, _, err := p.Run(m)
	if err != nil {
		return "", err
	}
	if res.Canceled {
		return "(canceled)", nil
	}
	choice, ok := m.Selected()
	if !ok {
		return "(nothing selected)", nil
	}
	return choice, nil
}
