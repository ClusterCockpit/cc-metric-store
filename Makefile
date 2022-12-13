
APP = cc-metric-store
GOSRC_APP        := cc-metric-store.go
GOSRC_FILES      := api.go \
		    memstore.go \
		    archive.go \
		    debug.go \
		    float.go \
		    lineprotocol.go \
		    selector.go \
		    stats.go



BINDIR ?= bin


.PHONY: all
all: $(APP)

$(APP): $(GOSRC)
	go get
	go build -o $(APP) $(GOSRC_APP) $(GOSRC_FILES)

install: $(APP)
	@WORKSPACE=$(PREFIX)
	@if [ -z "$${WORKSPACE}" ]; then exit 1; fi
	@mkdir --parents --verbose $${WORKSPACE}/usr/$(BINDIR)
	@install -Dpm 755 $(APP) $${WORKSPACE}/usr/$(BINDIR)/$(APP)
	@install -Dpm 600 config.json $${WORKSPACE}/etc/$(APP)/$(APP).json

.PHONY: clean
.ONESHELL:
clean:
	rm -f $(APP)

.PHONY: fmt
fmt:
	go fmt $(GOSRC_APP)

# Examine Go source code and reports suspicious constructs
.PHONY: vet
vet:
	go vet ./...

# Run linter for the Go programming language.
# Using static analysis, it finds bugs and performance issues, offers simplifications, and enforces style rules
.PHONY: staticcheck
staticcheck:
	go install honnef.co/go/tools/cmd/staticcheck@latest
	$$(go env GOPATH)/bin/staticcheck ./...

.ONESHELL:
.PHONY: RPM
RPM: scripts/cc-metric-store.spec
	@WORKSPACE="$${PWD}"
	@SPECFILE="$${WORKSPACE}/scripts/cc-metric-store.spec"
	# Setup RPM build tree
	@eval $$(rpm --eval "ARCH='%{_arch}' RPMDIR='%{_rpmdir}' SOURCEDIR='%{_sourcedir}' SPECDIR='%{_specdir}' SRPMDIR='%{_srcrpmdir}' BUILDDIR='%{_builddir}'")
	@mkdir --parents --verbose "$${RPMDIR}" "$${SOURCEDIR}" "$${SPECDIR}" "$${SRPMDIR}" "$${BUILDDIR}"
	# Create source tarball
	@COMMITISH="HEAD"
	@VERS=$$(git describe --tags $${COMMITISH})
	@VERS=$${VERS#v}
	@VERS=$$(echo $$VERS | sed -e s+'-'+'_'+g)
	@if [ "$${VERS}" = "" ]; then VERS="0.0.1"; fi
	@eval $$(rpmspec --query --queryformat "NAME='%{name}' VERSION='%{version}' RELEASE='%{release}' NVR='%{NVR}' NVRA='%{NVRA}'" --define="VERS $${VERS}" "$${SPECFILE}")
	@PREFIX="$${NAME}-$${VERSION}"
	@FORMAT="tar.gz"
	@SRCFILE="$${SOURCEDIR}/$${PREFIX}.$${FORMAT}"
	@git archive --verbose --format "$${FORMAT}" --prefix="$${PREFIX}/" --output="$${SRCFILE}" $${COMMITISH}
	# Build RPM and SRPM
	@rpmbuild -ba --define="VERS $${VERS}" --rmsource --clean "$${SPECFILE}"
	# Report RPMs and SRPMs when in GitHub Workflow
	@if [[ "$${GITHUB_ACTIONS}" == true ]]; then
	@     RPMFILE="$${RPMDIR}/$${ARCH}/$${NVRA}.rpm"
	@     SRPMFILE="$${SRPMDIR}/$${NVR}.src.rpm"
	@     echo "RPM: $${RPMFILE}"
	@     echo "SRPM: $${SRPMFILE}"
	@     echo "::set-output name=SRPM::$${SRPMFILE}"
	@     echo "::set-output name=RPM::$${RPMFILE}"
	@fi

.ONESHELL:
.PHONY: DEB
DEB: scripts/cc-metric-store.deb.control $(APP)
	@BASEDIR=$${PWD}
	@WORKSPACE=$${PWD}/.dpkgbuild
	@DEBIANDIR=$${WORKSPACE}/debian
	@DEBIANBINDIR=$${WORKSPACE}/DEBIAN
	@mkdir --parents --verbose $$WORKSPACE $$DEBIANBINDIR
	#@mkdir --parents --verbose $$DEBIANDIR
	@CONTROLFILE="$${BASEDIR}/scripts/cc-metric-store.deb.control"
	@COMMITISH="HEAD"
	@VERS=$$(git describe --tags --abbrev=0 $${COMMITISH})
	@VERS=$${VERS#v}
	@VERS=$$(echo $$VERS | sed -e s+'-'+'_'+g)
	@if [ "$${VERS}" = "" ]; then VERS="0.0.1"; fi
	@ARCH=$$(uname -m)
	@ARCH=$$(echo $$ARCH | sed -e s+'_'+'-'+g)
	@if [ "$${ARCH}" = "x86-64" ]; then ARCH=amd64; fi
	@PREFIX="$${NAME}-$${VERSION}_$${ARCH}"
	@SIZE_BYTES=$$(du -bcs --exclude=.dpkgbuild "$$WORKSPACE"/ | awk '{print $$1}' | head -1 | sed -e 's/^0\+//')
	@SIZE="$$(awk -v size="$$SIZE_BYTES" 'BEGIN {print (size/1024)+1}' | awk '{print int($$0)}')"
	#@sed -e s+"{VERSION}"+"$$VERS"+g -e s+"{INSTALLED_SIZE}"+"$$SIZE"+g -e s+"{ARCH}"+"$$ARCH"+g $$CONTROLFILE > $${DEBIANDIR}/control
	@sed -e s+"{VERSION}"+"$$VERS"+g -e s+"{INSTALLED_SIZE}"+"$$SIZE"+g -e s+"{ARCH}"+"$$ARCH"+g $$CONTROLFILE > $${DEBIANBINDIR}/control
	@make PREFIX=$${WORKSPACE} install
	@DEB_FILE="cc-metric-store_$${VERS}_$${ARCH}.deb"
	@dpkg-deb -b $${WORKSPACE} "$$DEB_FILE"
	@rm -r "$${WORKSPACE}"
	@if [ "$${GITHUB_ACTIONS}" = "true" ]; then
	@     echo "::set-output name=DEB::$${DEB_FILE}"
	@fi
