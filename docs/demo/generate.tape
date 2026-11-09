# Records docs/demo/generate.gif from a real run against the GitHub API:
#   GITHUB_TOKEN=... vhs docs/demo/generate.tape
Output docs/demo/generate.gif
Set WaitTimeout 180s
Set WaitPattern /\$\s*$/
Set Shell "bash"
Set FontSize 15
Set Width 1300
Set Height 620
Set Padding 18
Set Theme "Catppuccin Mocha"
Set TypingSpeed 30ms

Hide
Type "export PS1='$ ' && make build >/dev/null && clear" Enter
Sleep 3s
Show
Type "./bin/profile-forge generate --config examples/adeoluo.yaml --out /tmp/pf/README.md --json /tmp/pf/portfolio.json --diagrams-dir assets --mermaid-config examples/mermaid.config.json --check-links" Enter
Wait
Sleep 2s
Type `ls /tmp/pf/assets && grep -n 'img src="assets' /tmp/pf/README.md` Enter
Wait
Sleep 2s
