package v1alpha1

// Accessor methods shared by all product CRs so controllers can treat them
// uniformly (see controllers.ProductObject).

func (p *PingFederate) GetEnvironmentRef() string { return p.Spec.EnvironmentRef }
func (p *PingFederate) SetPhase(phase string)     { p.Status.Phase = phase }

func (p *PingDirectory) GetEnvironmentRef() string { return p.Spec.EnvironmentRef }
func (p *PingDirectory) SetPhase(phase string)     { p.Status.Phase = phase }

func (p *PingAccess) GetEnvironmentRef() string { return p.Spec.EnvironmentRef }
func (p *PingAccess) SetPhase(phase string)     { p.Status.Phase = phase }

func (p *PingAuthorize) GetEnvironmentRef() string { return p.Spec.EnvironmentRef }
func (p *PingAuthorize) SetPhase(phase string)     { p.Status.Phase = phase }

func (p *PingAuthorizePAP) GetEnvironmentRef() string { return p.Spec.EnvironmentRef }
func (p *PingAuthorizePAP) SetPhase(phase string)     { p.Status.Phase = phase }

func (p *PingDataSync) GetEnvironmentRef() string { return p.Spec.EnvironmentRef }
func (p *PingDataSync) SetPhase(phase string)     { p.Status.Phase = phase }

func (p *PingDirectoryProxy) GetEnvironmentRef() string { return p.Spec.EnvironmentRef }
func (p *PingDirectoryProxy) SetPhase(phase string)     { p.Status.Phase = phase }
