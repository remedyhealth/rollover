BIN_DIR := bin
CMD_DIR := cmd
SHARED_DIR := shared
FANOUT_NAME := fanout
REFRESH_NAME := refresh

SHARED_SRC := $(filter-out %_test.go,$(wildcard $(SHARED_DIR)/*.go))
FANOUT_SRC := $(filter-out %_test.go,$(wildcard $(CMD_DIR)/$(FANOUT_NAME)/*.go))
REFRESH_SRC := $(filter-out %_test.go,$(wildcard $(CMD_DIR)/$(REFRESH_NAME)/*.go))

LDFLAGS := -X 'github.com/remedyhealth/rollover/shared.Version=${CIRCLE_TAG}'
LDFLAGS += -X 'github.com/remedyhealth/rollover/shared.BuildNum=${CIRCLE_BUILD_NUM}'
LDFLAGS += -X 'github.com/remedyhealth/rollover/shared.Rev=${CIRCLE_SHA1}'

define gobuild
go build -ldflags "$(LDFLAGS)" -o $@ ./$(dir $<)
endef

.PHONY: all
all: fanout refresh

.PHONY: fanout
fanout: $(BIN_DIR)/$(FANOUT_NAME)

.PHONY: refresh
refresh: $(BIN_DIR)/$(REFRESH_NAME)

.PHONY: clean
clean:
	rm -rf bin

.PHONY: test
test: ;

$(BIN_DIR)/$(FANOUT_NAME): $(FANOUT_SRC) $(SHARED_SRC)
	$(gobuild)

$(BIN_DIR)/$(REFRESH_NAME): $(REFRESH_SRC) $(SHARED_SRC)
	$(gobuild)
