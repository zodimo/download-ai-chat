.PHONY: help version validate-version version-bump-patch version-bump-minor version-bump-major


# Show this help text. Running `make` with no target is equivalent to `make help`.
.DEFAULT_GOAL := help
help: ## Display available targets and descriptions
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)


VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# Version without the leading "v". Release tags (v1.2.3 -> 1.2.3); anything
# else (untagged describe output) falls back to "dev".
CLEAN_VERSION = $(if $(filter v%,$(VERSION)),$(patsubst v%,%,$(VERSION)),dev)

# Highest vX.Y.Z tag (version-sorted), else 0.0.0.
CURRENT_VERSION := $(shell git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | head -1 | sed 's/^v//')
ifeq ($(CURRENT_VERSION),)
CURRENT_VERSION := 0.0.0
endif
MAJOR := $(word 1,$(subst ., ,$(CURRENT_VERSION)))
MINOR := $(word 2,$(subst ., ,$(CURRENT_VERSION)))
PATCH := $(word 3,$(subst ., ,$(CURRENT_VERSION)))

NEXT_PATCH_VERSION := $(MAJOR).$(MINOR).$(shell echo $$(( $(PATCH) + 1 )))
NEXT_MINOR_VERSION := $(MAJOR).$(shell echo $$(( $(MINOR) + 1 ))).0
NEXT_MAJOR_VERSION := $(shell echo $$(( $(MAJOR) + 1 ))).0.0

# Strict semver (optional pre-release + build metadata suffixes).
VERSION_VALID := $(shell echo "$(CURRENT_VERSION)" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$$' && echo valid || echo invalid)
IS_DIRTY := $(shell git diff-index --quiet HEAD -- 2>/dev/null && echo clean || echo dirty)

# Show the current release version (highest semver tag, else 0.0.0), the
# effective build version, the active build env/mode knobs, and the derived
# artifact directory.
version: ## Show current/effective version + artifact dir
	@echo "Current version: $(CURRENT_VERSION)"
	@echo "Effective build version: v$(CLEAN_VERSION)"

# Validate the current release version against the semver spec.
validate-version: ## Fail if the current tag is not valid semver
	@if [ "$(VERSION_VALID)" = "valid" ]; then \
		echo "Version $(CURRENT_VERSION) is valid semver"; \
	else \
		echo "ERROR: current version '$(CURRENT_VERSION)' is not valid semver" >&2; \
		exit 1; \
	fi

# Create annotated tags bumping the patch/minor/major component.
version-bump-patch: ## Create annotated tag v<current>.<current.minor>.<current.patch+1>
	$(check-dirty)
	git tag -a "v$(NEXT_PATCH_VERSION)" -m "Release v$(NEXT_PATCH_VERSION)"
	@echo "Tagged $(CURRENT_VERSION) -> v$(NEXT_PATCH_VERSION)"

version-bump-minor: ## Create annotated tag v<current>.<current.minor+1>.0
	$(check-dirty)
	git tag -a "v$(NEXT_MINOR_VERSION)" -m "Release v$(NEXT_MINOR_VERSION)"
	@echo "Tagged $(CURRENT_VERSION) -> v$(NEXT_MINOR_VERSION)"

version-bump-major: ## Create annotated tag v<current+1>.0.0
	$(check-dirty)
	git tag -a "v$(NEXT_MAJOR_VERSION)" -m "Release v$(NEXT_MAJOR_VERSION)"
	@echo "Tagged $(CURRENT_VERSION) -> v$(NEXT_MAJOR_VERSION)"