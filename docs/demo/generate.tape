# Records docs/demo/generate.gif from a real run against the GitHub API:
#   GITHUB_TOKEN=... vhs docs/demo/generate.tape
Output docs/demo/generate.gif
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
Type "./bin/profile-forge generate --config examples/adeoluo.yaml --out /tmp/README.md --json /tmp/portfolio.json --check-links" Enter
Sleep 12s
Type "head -n 14 /tmp/README.md" Enter
Sleep 4s
