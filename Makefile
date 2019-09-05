release:
	export GITHUB_TOKEN=;\
	goreleaser --config=./.goreleaser.yml --rm-dist