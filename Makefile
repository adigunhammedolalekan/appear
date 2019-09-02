release:
	export GITHUB_TOKEN=03038bf4f65ed6400c2a0f4e4004f40bafa1e3f5;\
	goreleaser --config=./.goreleaser.yml --snapshot --rm-dist