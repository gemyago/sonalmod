include build/make/golangci-lint.mk

cover_dir=.cover
cover_profile=$(cover_dir)/profile.out

go-test-coverage=go run github.com/vladopajic/go-test-coverage/v2

$(cover_dir):
	mkdir -p $(cover_dir)

$(cover_dir)/repo-name-with-owner.txt:
	gh repo view --json nameWithOwner -q .nameWithOwner > $@

$(cover_dir)/coverage.%.blob-sha: $(cover_dir)/repo-name-with-owner.txt
	gh api \
		--method GET \
		-H "Accept: application/vnd.github+json" \
		-H "X-GitHub-Api-Version: 2022-11-28" \
		/repos/$(shell cat $(cover_dir)/repo-name-with-owner.txt)/contents/coverage/golang-coverage.$*?ref=test-artifacts \
		| jq -jr '.sha' > $@

$(cover_dir)/coverage.%.gh-cli-body.json: $(cover_dir)/coverage.% $(cover_dir)/coverage.%.blob-sha
	@echo "{" > $@
	@echo "\"branch\": \"test-artifacts\"," >> $@
	@printf "\"sha\": \"">> $@
	@cat $(cover_dir)/coverage.$*.blob-sha >> $@
	@printf "\",\n">> $@
	@echo "\"message\": \"Updating golang coverage.$*\",">> $@
	@printf "\"content\": \"">> $@
	@base64 -i $< | tr -d '\n' >> $@
	@printf "\"\n}">> $@

# Orphan branch will need to be created prior to running this
# git checkout --orphan test-artifacts
# git rm -rf .
# rm -f .gitignore
# echo '# Test Artifacts' > README.md
# git add README.md
# git commit -m 'init'
# git push origin test-artifacts
.PHONY: push-test-artifacts
push-test-artifacts: $(cover_dir)/coverage.svg.gh-cli-body.json $(cover_dir)/coverage.html.gh-cli-body.json $(cover_dir)/repo-name-with-owner.txt
	@gh api \
		--method PUT \
		-H "Accept: application/vnd.github+json" \
		-H "X-GitHub-Api-Version: 2022-11-28" \
		/repos/$(shell cat $(cover_dir)/repo-name-with-owner.txt)/contents/coverage/golang-coverage.svg \
		--input $(cover_dir)/coverage.svg.gh-cli-body.json
	@gh api \
		--method PUT \
		-H "Accept: application/vnd.github+json" \
		-H "X-GitHub-Api-Version: 2022-11-28" \
		/repos/$(shell cat $(cover_dir)/repo-name-with-owner.txt)/contents/coverage/golang-coverage.html \
		--input $(cover_dir)/coverage.html.gh-cli-body.json

.PHONY: affected-lint-test
ifeq ($(CI),true)
affected-lint-test:
	npx nx affected -t lint test --tuiAutoExit
else
affected-lint-test: $(GOLANGCI_LINT_BIN)
	npx nx affected -t lint test --tuiAutoExit
endif

.PHONY: lint
lint:
	$(MAKE) -C tools/firecrawl lint
	$(MAKE) -C runtime lint
	$(MAKE) -C apps/sonalmod lint
	$(MAKE) -C apps/sonal-ui lint
	$(MAKE) -C tools/workspacefs lint
	$(MAKE) -C tools/skills lint

.PHONY: clean-lint-cache
clean-lint-cache:
	rm -r -f .cache/golangci-lint

# We will need to rework coverage collection once we have more than one module.
.PHONY: test
test: $(cover_dir)
	rm -r -f $(cover_dir)/*
	$(MAKE) -C tools/firecrawl test
	$(MAKE) -C tools/skills test
	$(MAKE) -C tools/workspacefs test
	$(MAKE) -C runtime test
	$(MAKE) -C apps/sonalmod test
	$(MAKE) -C apps/sonal-ui test
	cat tools/firecrawl/.cover/profile.out > $(cover_profile)
	tail -n +2 tools/skills/.cover/profile.out >> $(cover_profile)
	tail -n +2 tools/workspacefs/.cover/profile.out >> $(cover_profile)
	tail -n +2 runtime/.cover/profile.out >> $(cover_profile)
	tail -n +2 apps/sonalmod/.cover/profile.out >> $(cover_profile)
	go tool cover -html=$(cover_profile) -o $(cover_dir)/coverage.html
	$(go-test-coverage) --badge-file-name $(cover_dir)/coverage.svg --profile $(cover_profile)
