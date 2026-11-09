# Records docs/demo/check.gif from a real run: `vhs docs/demo/check.tape` from the repo root.
Output docs/demo/check.gif
Set WaitTimeout 180s
Set WaitPattern /\$\s*$/
Set Shell "bash"
Set FontSize 15
Set Width 1300
Set Height 420
Set Padding 18
Set Theme "Catppuccin Mocha"
Set TypingSpeed 22ms

Hide
Type `export PS1='$ ' && make build >/dev/null && clear` Enter
Sleep 10s
Show
Type `./bin/profile-forge check --config examples/adeoluo.yaml --readme examples/adeoluo.README.md --diagrams-dir assets` Enter
Wait
Sleep 2s
