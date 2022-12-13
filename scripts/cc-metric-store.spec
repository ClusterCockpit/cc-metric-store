Name:           cc-metric-store
Version:        %{VERS}
Release:        1%{?dist}
Summary:        In-memory metric database from the ClusterCockpit suite

License:        MIT
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  go-toolset
BuildRequires:  systemd-rpm-macros

Provides:       %{name} = %{version}

%description
In-memory metric database from the ClusterCockpit suite

%global debug_package %{nil}

%prep
%autosetup


%build
make


%install
# Install cc-metric-store
make PREFIX=%{buildroot} install
# Integrate into system
install -Dpm 0644 scripts/%{name}.service %{buildroot}%{_unitdir}/%{name}.service
install -Dpm 0600 scripts/%{name}.config %{buildroot}%{_sysconfdir}/default/%{name}
install -Dpm 0644 scripts/%{name}.sysusers %{buildroot}%{_sysusersdir}/%{name}.conf


%check
# go test should be here... :)

%pre
%sysusers_create_package scripts/%{name}.sysusers

%post
%systemd_post %{name}.service

%preun
%systemd_preun %{name}.service

%files
# Binary
%attr(-,clustercockpit,clustercockpit) %{_bindir}/%{name}
# Config
%dir %{_sysconfdir}/%{name}
%attr(0600,clustercockpit,clustercockpit) %config(noreplace) %{_sysconfdir}/%{name}/%{name}.json
# Systemd
%{_unitdir}/%{name}.service
%{_sysconfdir}/default/%{name}
%{_sysusersdir}/%{name}.conf

%changelog
* Mon Mar 07 2022 Thomas Gruber - 0.1
- Initial metric store implementation

