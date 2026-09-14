# Checks the recipe tree with a released dispelnet, locally and in CI.
#
#   make check                           newest dispelnet release
#   make check DISPELNET_VERSION=v0.1.0  a given release
#   make check DISPELNET=/path/to/dispelnet
#
# Fetching a release needs the gh CLI, authenticated.

BUILD      := build
DISPELNET  ?= $(BUILD)/dispelnet
ACTIONLINT := github.com/rhysd/actionlint/cmd/actionlint@v1.7.7
TREE       ?= $(BUILD)/tree

.PHONY: check tree verify verify-tree lint clean

check: lint verify

# Checks the layout of recipes/ and copies it into the tree dispelnet reads.
tree:
	rm -rf $(BUILD)/tree
	bash .github/scripts/collect-tree.sh . $(BUILD)/tree

$(BUILD)/dispelnet:
	bash .github/scripts/fetch-dispelnet.sh $@ $(DISPELNET_VERSION)

verify: tree
	$(MAKE) --no-print-directory verify-tree TREE=$(BUILD)/tree

# The one place that knows how a tree is checked, so the release workflow can
# check its unpacked tarball the same way: --from names the recipes, and the
# positional argument names where the captures are. Without the second one,
# verify parses the recipes and reads no device output at all.
verify-tree: $(DISPELNET)
	$(DISPELNET) recipes verify --from $(TREE) $(TREE)

lint:
	bash .github/scripts/check-pins.sh
	for script in .github/scripts/*.sh; do bash -n "$$script" || exit 1; done
	go run $(ACTIONLINT)

clean:
	rm -rf $(BUILD)
