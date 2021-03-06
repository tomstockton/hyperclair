package clair

import (
	"strconv"
	"strings"
  "math"
	"sort"

	"github.com/coreos/clair/api/v1"
	"github.com/spf13/viper"
	"github.com/wemanity-belgium/hyperclair/xstrings"
)

var uri string
var priority string
var healthPort int

//Report Reporting Config value
var Report ReportConfig

//VulnerabiliesCounts Total count of vulnerabilities
type VulnerabiliesCounts struct {
  Total  int
  High   int
  Medium int
  Low    int
	Negligible int
  TotalFeatures int
  SafeFeatures int
  UnsafeFeatures int
}

//RelativeCount get the percentage of vulnerabilities of a severity
func (vulnerabilityCount VulnerabiliesCounts) RelativeCount(severity string, useFeatureNumber bool) float64  {
  var count int
  
  switch severity {
  case "High":
    count = vulnerabilityCount.High
  case "Medium":
    count = vulnerabilityCount.Medium
  case "Low":
    count = vulnerabilityCount.Low
  }
  
  result := float64(count) / float64(vulnerabilityCount.Total) * 100
  
  if (useFeatureNumber) {
    relativeCorrection := float64(vulnerabilityCount.UnsafeFeatures) / float64(vulnerabilityCount.TotalFeatures)
    result = result * relativeCorrection
  }
  
  return math.Ceil(result * 100) / 100
}

//ImageAnalysis Full image analysis
type ImageAnalysis struct {
	Registry  string
	ImageName string
	Tag       string
	Layers    []v1.LayerEnvelope
}

func (imageAnalysis ImageAnalysis) String() string {
	return imageAnalysis.Registry + "/" + imageAnalysis.ImageName + ":" + imageAnalysis.Tag
}

func (imageAnalysis ImageAnalysis) ShortName(l v1.Layer) string {
	return xstrings.Substr(l.Name, 0, 12)
}

// CountVulnerabilities counts all image vulnerability
func (imageAnalysis ImageAnalysis) CountVulnerabilities(l v1.Layer) int {
	count := 0
	for _, f := range l.Features {
		count += len(f.Vulnerabilities)
	}
	return count
}

// CountAllVulnerabilities Total count of vulnerabilities
func (imageAnalysis ImageAnalysis) CountAllVulnerabilities() VulnerabiliesCounts {
  var result VulnerabiliesCounts;
  result.Total = 0
  result.High = 0
  result.Medium = 0
  result.Low = 0
	result.Negligible = 0
  result.TotalFeatures = 0
  result.SafeFeatures = 0
  
  for _, l := range imageAnalysis.Layers {
    result.TotalFeatures += len(l.Layer.Features)
    
    for _, f := range l.Layer.Features {
      if len(f.Vulnerabilities) != 0 {
        result.UnsafeFeatures++
      }
      
      result.Total += len(f.Vulnerabilities)

      for _, v := range f.Vulnerabilities {
        switch v.Severity {
        case "High":
          result.High++
        case "Medium":
          result.Medium++
        case "Low":
          result.Low++
				case "Negligible":
					result.Negligible++
        }
      }
    }
  }
  
  result.SafeFeatures = result.TotalFeatures - result.UnsafeFeatures

  return result;
}

// Vulnerability : A vulnerability inteface
type Vulnerability struct {
	Name, Severity, IntroduceBy, Description, Link, Layer string
}

// Weight get the weight of the vulnerability according to its Severity
func (v Vulnerability) Weight() int  {
	weight := 0
	
	switch v.Severity {
		case "High":
			weight = 4
		case "Medium":
			weight = 3
		case "Low":
			weight = 2
		case "Negligible":
			weight = 1
		}
	
	return weight
}

// Layer : A layer inteface
type Layer struct {
	Name string
	Path string
	Namespace string
	Features []Feature
}

// Feature : A feature inteface
type Feature struct {
	Name string
	Version string
	Vulnerabilities []Vulnerability
}

// Status give the healthy / unhealthy statut of a feature
func (feature Feature) Status() bool  {
	return len(feature.Vulnerabilities) == 0;
}

// Weight git the weight of a featrure according to its vulnerabilities
func (feature Feature) Weight() int {
	weight := 0
	
	for _, v := range feature.Vulnerabilities {
		weight += v.Weight()
	}
	
	return weight
}

// VulnerabilitiesBySeverity sorting vulnerabilities by severity
type VulnerabilitiesBySeverity []Vulnerability

func (a VulnerabilitiesBySeverity) Len() int { return len(a) }
func (a VulnerabilitiesBySeverity) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a VulnerabilitiesBySeverity) Less(i, j int) bool  {
	return a[i].Weight() > a[j].Weight()
}

// LayerByVulnerabilities sorting of layers by global vulnerability
type LayerByVulnerabilities []Layer

func (a LayerByVulnerabilities) Len() int { return len(a) }
func (a LayerByVulnerabilities) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a LayerByVulnerabilities) Less(i, j int) bool {
	firstVulnerabilities := 0
	secondVulnerabilities := 0
	
	for _, l := range a[i].Features {
		firstVulnerabilities = firstVulnerabilities + l.Weight()
	}
	
	for _ , l := range a[j].Features {
		secondVulnerabilities = secondVulnerabilities + l.Weight()
	}
	
	return firstVulnerabilities > secondVulnerabilities
}

// FeatureByVulnerabilities sorting off features by vulnerabilities
type FeatureByVulnerabilities []Feature

func (a FeatureByVulnerabilities) Len() int { return len(a) }
func (a FeatureByVulnerabilities) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

func (a FeatureByVulnerabilities) Less(i, j int) bool {
	return a[i].Weight() > a[j].Weight()
}

// SortLayers give layers ordered by vulnerability algorithm
func (imageAnalysis ImageAnalysis) SortLayers() []Layer {
	layers := []Layer{}
	
	for _, l := range imageAnalysis.Layers {
		features := []Feature{}
		
		for _, f := range l.Layer.Features {
			vulnerabilities := []Vulnerability{}
			
			for _, v := range f.Vulnerabilities {
				nv := Vulnerability{
					Name:        v.Name,
					Severity:    v.Severity,
					IntroduceBy: f.Name + ":" + f.Version,
					Description: v.Description,
					Layer:       l.Layer.Name,
					Link:        v.Link,
				}
				
				vulnerabilities = append(vulnerabilities, nv);
			}
			
			sort.Sort(VulnerabilitiesBySeverity(vulnerabilities))
			
			nf := Feature{
				Name: f.Name,
				Version: f.Version,
				Vulnerabilities: vulnerabilities,
			}
			
			features = append(features, nf);
		}
		
		sort.Sort(FeatureByVulnerabilities(features))
		
		nl := Layer{
			Name: l.Layer.Name,
			Path: l.Layer.Path,
			Features: features,
		}
		layers = append(layers, nl);
	}
	
	sort.Sort(LayerByVulnerabilities(layers));
	
	return layers;
}

// SortVulnerabilities get all vulnerabilities sorted by Severity
func (imageAnalysis ImageAnalysis) SortVulnerabilities() []Vulnerability {
	vulnerabilities := []Vulnerability{}
	
	// there should be a better method, but I don't know how to easlily concert []v1.Vulnerability to [Vulnerability]
	for _, l := range imageAnalysis.Layers {
		for _, f := range l.Layer.Features {
			for _, v := range f.Vulnerabilities {
				nv := Vulnerability{
					Name:        v.Name,
					Severity:    v.Severity,
					IntroduceBy: f.Name + ":" + f.Version,
					Description: v.Description,
					Layer:       l.Layer.Name,
				}
				
				vulnerabilities = append(vulnerabilities, nv)
			}
		}
	}
	
	sort.Sort(VulnerabilitiesBySeverity(vulnerabilities));

	return vulnerabilities
}

func fmtURI(u string, port int) {
	uri = u
	if port != 0 {
		uri += ":" + strconv.Itoa(port)
	}
	if !strings.HasSuffix(uri, "/v1") {
		uri += "/v1"
	}
	if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
		uri = "http://" + uri
	}
}

//Config configure Clair from configFile
func Config() {
	fmtURI(viper.GetString("clair.uri"), viper.GetInt("clair.port"))
	priority = viper.GetString("clair.priority")
	healthPort = viper.GetInt("clair.healthPort")
	Report.Path = viper.GetString("clair.report.path")
	Report.Format = viper.GetString("clair.report.format")
}