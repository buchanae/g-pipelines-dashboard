package hello

import (
    "fmt"
    "net/http"
    "text/template"
    "time"
    "strings"
    "golang.org/x/oauth2/google"
    "google.golang.org/api/genomics/v1"
    "google.golang.org/appengine"
    "encoding/json"
    "os"
)

func init() {
    http.HandleFunc("/", handler)
}

func handler(w http.ResponseWriter, r *http.Request) {
    ctx := appengine.NewContext(r)

    project := os.Getenv("PROJECT")

    if project == "" {
      project = appengine.AppID(ctx)
    }

    if project == "None" {
      fmt.Fprintln(w, "no project found")
      return
    }

    client, err := google.DefaultClient(ctx, genomics.GenomicsScope)
    if err != nil {
      fmt.Fprintln(w, err.Error())
      return
    }

    svc, err := genomics.New(client)
    if err != nil {
      fmt.Fprintln(w, err.Error())
      return
    }


    w.Header().Add("content-type", "text/html")

    ops := genomics.NewOperationsService(svc)
    resp, err := ops.List("operations").Filter("projectId = " + project).Do()
    if err != nil {
      fmt.Fprintln(w, err.Error())
      return
    }

    var tplOps []tplOp

    for _, op := range resp.Operations {

      meta := genomics.OperationMetadata{}
      err := json.Unmarshal(op.Metadata, &meta)
      if err != nil {
        fmt.Fprintln(w, err.Error())
        return
      }

      if meta.StartTime == "" {
        continue
      }
      startTime, _ := time.Parse(time.RFC3339, meta.StartTime)

      var endTime time.Time
      if meta.EndTime == "" {
        endTime = time.Now()
      } else {
        endTime, _ = time.Parse(time.RFC3339, meta.EndTime)
      }

      dur := endTime.Sub(startTime)

      // GCE bills at a minimum of 1 minute
      if dur < time.Minute {
        dur = time.Minute
      }

      hours := float64(dur) / float64(time.Hour)

      runtime := genomics.RuntimeMetadata{}
      json.Unmarshal(meta.RuntimeMetadata, &runtime)
      gce := runtime.ComputeEngine
      hourly, ok := hourlyVMPrices[gce.MachineType]

      cost := ""
      if !ok {
        cost = "unknown"
      } else {
        cost = fmt.Sprintf("%f", hours * hourly)
      }

      tplOps = append(tplOps, tplOp{
        Name: strings.TrimPrefix(op.Name, "operations/")[:10],
        Meta: meta,
        GCE: gce,
        Duration: dur,
        Hourly: hourly,
        Hours: hours,
        Cost: cost,
      })
    }

    err = tpl.Execute(w, struct {
      Ops []tplOp
      Prices map[string]float64
      Project string
    }{
      Ops: tplOps,
      Prices: hourlyVMPrices,
      Project: project,
    })
    if err != nil {
      fmt.Fprintln(w, err.Error())
      return
    }
}

type tplOp struct {
  Name string
  Meta genomics.OperationMetadata
  GCE *genomics.ComputeEngine
  Duration time.Duration
  Hourly float64
  Hours float64
  Cost string
}

var tpl = template.Must(template.New("page").Parse(`
<h1>Google Pipelines Cost Dashboard for Project "{{.Project}}"</h1>

<h2>Operations</h2>
<table>
<thead>
  <th>Name</th>
  <th>Duration</th>
  <th>Machine Type</th>
  <th>Hours Billed</th>
  <th>Cost</th>
</thead>
<tbody>
  {{ range $index, $el := .Ops }}
  <tr>
    <td>{{ $el.Name }}</td>
    <td>{{ $el.Duration }}</td>
    <td>{{ $el.GCE.MachineType }}</td>
    <td>{{ $el.Hours }}</td>
    <td>{{ $el.Cost }}</td>
  </tr>
  {{ end }}
</tbody>
</table>

<h2>Prices</h2>

<table>
<thead>
  <th>
    machine
  </th>
  <th>
    hourly price
  </th>
</thead>
<tbody>
  {{ range $index, $el := .Prices }}
  <tr>
    <td>{{ $index }}</td>
    <td>{{ $el }}</td>
  </tr>
  {{ end }}
</tbody>
</table>
`))


var regions = strings.Fields(`
us
us-central1
us-east1
us-east4
us-west1
europe
europe-west1
europe-west2
europe-west3
asia
asia-east
asia-northeast
asia-southeast
australia
australia-northeast
australia-southeast
`)

var zones = strings.Fields("a b c d e f")

var mixedPriceData struct {
  Version string
  Updated string
  PriceList map[string]interface{} `json:"gcp_price_list"`
}

var hourlyVMPrices = map[string]float64{}

func init() {
  err := json.Unmarshal([]byte(rawPriceData), &mixedPriceData)
  if err != nil {
    panic(err)
  }


  for k, i := range mixedPriceData.PriceList {
    if strings.HasPrefix(k, "CP-COMPUTEENGINE-VMIMAGE-") {
      vm := strings.TrimPrefix(k, "CP-COMPUTEENGINE-VMIMAGE-")
      vm = strings.ToLower(vm)
      hourlyVMPrices[vm] = 0

      dat := i.(map[string]interface{})
      for _, r := range regions {
        if v, ok := dat[r]; ok {
          price := v.(float64)
          for _, z := range zones {
            hourlyVMPrices[r + "-" + z + "/" + vm] = price
          }
        }
      }
    }
  }
}

var rawPriceData = `
{
  "comment": "If you've gotten here by mistake, this is the JSON data used by our pricing calculator. It is helpful for developers. Go to https://cloud.google.com/products/calculator/ to get back to our web calculator.",
  "version": "v1.21",
  "updated": "12-December-2017",
  "gcp_price_list": {
    "sustained_use_base": 0.25,
    "sustained_use_tiers": {
      "0.25": 1.0,
      "0.50": 0.8,
      "0.75": 0.6,
      "1.0": 0.4
    },
    "CP-COMPUTEENGINE-VMIMAGE-F1-MICRO": {
      "us": 0.0076,
      "us-central1": 0.0076,
      "us-east1": 0.0076,
      "us-east4": 0.0086,
      "us-west1": 0.0076,
      "europe": 0.0086,
      "europe-west1": 0.0086,
      "europe-west2": 0.0096,
      "europe-west3": 0.0096,
      "asia": 0.0090,
      "asia-east": 0.0090,
      "asia-northeast": 0.0092,
      "asia-southeast": 0.0092,
      "australia-southeast1": 0.0106,
      "australia": 0.0106,
      "southamerica-east1": 0.0118,
      "asia-south1": 0.0091,
      "cores": "shared",
      "memory": "0.6",
      "gceu": "Shared CPU, not guaranteed",
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0]
    },
    "CP-COMPUTEENGINE-VMIMAGE-G1-SMALL": {
      "us": 0.027,
      "us-central1": 0.027,
      "us-east1": 0.027,
      "us-east4": 0.0289,
      "us-west1": 0.027,
      "europe": 0.0285,
      "europe-west1": 0.0285,
      "europe-west2": 0.0324,
      "europe-west3": 0.0324,
      "asia": 0.030,
      "asia-east": 0.030,
      "asia-northeast": 0.0322,
      "asia-southeast": 0.0311,
      "australia-southeast1": 0.0357,
      "australia": 0.0357,
      "southamerica-east1": 0.0400,
      "asia-south1": 0.0308,
      "cores": "shared",
      "memory": "1.7",
      "gceu": 1.38,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-1": {
      "us": 0.0475,
      "us-central1": 0.0475,
      "us-east1": 0.0475,
      "us-east4": 0.0535,
      "us-west1": 0.0475,
      "europe": 0.0523,
      "europe-west1": 0.0523,
      "europe-west2": 0.0612,
      "europe-west3": 0.0612,
      "asia": 0.055,
      "asia-east": 0.055,
      "asia-northeast": 0.0610,
      "asia-southeast": 0.0586,
      "australia-southeast1": 0.0674,
      "australia": 0.0674,
      "southamerica-east1": 0.0754,
      "asia-south1": 0.0570,
      "cores": "1",
      "memory": "3.75",
      "gceu": 2.75,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-2": {
      "us": 0.0950,
      "us-central1": 0.0950,
      "us-east1": 0.0950,
      "us-east4": 0.1070,
      "us-west1": 0.0950,
      "europe": 0.1046,
      "europe-west1": 0.1046,
      "europe-west2": 0.1224,
      "europe-west3": 0.1224,
      "asia": 0.110,
      "asia-east": 0.110,
      "asia-northeast": 0.1220,
      "asia-southeast": 0.1172,
      "australia-southeast1": 0.1348,
      "australia": 0.1348,
      "southamerica-east1": 0.1508,
      "asia-south1": 0.1141,
      "cores": "2",
      "memory": "7.5",
      "gceu": 5.50,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-4": {
      "us": 0.1900,
      "us-central1": 0.1900,
      "us-east1": 0.1900,
      "us-east4": 0.2140,
      "us-west1": 0.1900,
      "europe": 0.2092,
      "europe-west1": 0.2092,
      "europe-west2": 0.2448,
      "europe-west3": 0.2448,
      "asia": 0.220,
      "asia-east": 0.220,
      "asia-northeast": 0.2440,
      "asia-southeast": 0.2344,
      "australia-southeast1": 0.2697,
      "australia": 0.2697,
      "southamerica-east1": 0.3017,
      "asia-south1": 0.2282,
      "cores": "4",
      "memory": "15",
      "gceu": 11,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-8": {
      "us": 0.3800,
      "us-central1": 0.3800,
      "us-east1": 0.3800,
      "us-east4": 0.4280,
      "us-west1": 0.3800,
      "europe": 0.4184,
      "europe-west1": 0.4184,
      "europe-west2": 0.4896,
      "europe-west3": 0.4896,
      "asia": 0.440,
      "asia-east": 0.440,
      "asia-northeast": 0.4880,
      "asia-southeast": 0.4688,
      "australia-southeast1": 0.5393,
      "australia": 0.5393,
      "southamerica-east1": 0.6034,
      "asia-south1": 0.4564,
      "cores": "8",
      "memory": "30",
      "gceu": 22,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-16": {
      "us": 0.7600,
      "us-central1": 0.7600,
      "us-east1": 0.7600,
      "us-east4": 0.8560,
      "us-west1": 0.7600,
      "europe": 0.8368,
      "europe-west1": 0.8368,
      "europe-west2": 0.9792,
      "europe-west3": 0.9792,
      "asia": 0.880,
      "asia-east": 0.880,
      "asia-northeast": 0.9760,
      "asia-southeast": 0.9376,
      "australia-southeast1":1.0787,
      "australia":1.0787,
      "southamerica-east1": 1.2068,
      "asia-south1": 0.9127,
      "cores": "16",
      "memory": "60",
      "gceu": 44,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-32": {
      "us": 1.5200,
      "us-central1": 1.5200,
      "us-east1": 1.5200,
      "us-east4": 1.7120,
      "us-west1": 1.5200,
      "europe": 1.6736,
      "europe-west1": 1.6736,
      "europe-west2": 1.9584,
      "europe-west3": 1.9584,
      "asia": 1.760,
      "asia-east": 1.760,
      "asia-northeast": 1.9520,
      "asia-southeast": 1.8752,
      "australia-southeast1":2.1574,
      "australia":2.1574,
      "southamerica-east1": 2.4136,
      "asia-south1": 1.8255,
      "cores": "32",
      "memory": "120",
      "gceu": 88,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-64": {
      "us": 3.0400,
      "us-central1": 3.0400,
      "us-east1": 3.0400,
      "us-east4": 3.4240,
      "us-west1": 3.0400,
      "europe": 3.3472,
      "europe-west1": 3.3472,
      "europe-west2": 3.9168,
      "europe-west3": 3.9168,
      "asia": 3.520,
      "asia-east": 3.520,
      "asia-northeast": 3.9040,
      "asia-southeast": 3.7504,
      "australia-southeast1":4.3147,
      "australia":4.3147,
      "southamerica-east1": 4.8271,
      "asia-south1": 3.6510,
      "cores": "64",
      "memory": "240",
      "gceu": 176,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-96": {
      "us": 4.5600,
      "us-central1": 4.5600,
      "us-east1": 0,
      "us-east4": 0,
      "us-west1": 4.5600,
      "europe": 5.0208,
      "europe-west1": 5.0208,
      "europe-west2": 0,
      "europe-west3": 0,
      "asia": 5.2800,
      "asia-east": 5.2800,
      "asia-northeast": 0,
      "asia-southeast": 0,
      "australia-southeast1": 0,
      "australia": 0,
      "southamerica-east1": 0,
      "asia-south1": 0,
      "cores": "96",
      "memory": "360",
      "gceu": 264,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHMEM-2": {
      "us": 0.1184,
      "us-central1": 0.1184,
      "us-east1": 0.1184,
      "us-east4": 0.1348,
      "us-west1": 0.1184,
      "europe": 0.1302,
      "europe-west1": 0.1302,
      "europe-west2": 0.1523,
      "europe-west3": 0.1523,
      "asia": 0.1370,
      "asia-east": 0.1370,
      "asia-northeast": 0.1517,
      "asia-southeast": 0.1460,
      "australia-southeast1": 0.1679,
      "australia": 0.1679,
      "southamerica-east1": 0.1879,
      "asia-south1": 0.1421,
      "cores": "2",
      "memory": "13",
      "gceu": 5.50,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHMEM-4": {
      "us": 0.2368,
      "us-central1": 0.2368,
      "us-east1": 0.2368,
      "us-east4": 0.2696,
      "us-west1": 0.2368,
      "europe": 0.2604,
      "europe-west1": 0.2604,
      "europe-west2": 0.3046,
      "europe-west3": 0.3046,
      "asia": 0.2740,
      "asia-east": 0.2740,
      "asia-northeast": 0.3034,
      "asia-southeast": 0.2920,
      "australia-southeast1": 0.3358,
      "australia": 0.3358,
      "southamerica-east1": 0.3757,
      "asia-south1": 0.2842,
      "cores": "4",
      "memory": "26",
      "gceu": 11,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHMEM-8": {
      "us": 0.4736,
      "us-central1": 0.4736,
      "us-east1": 0.4736,
      "us-east4": 0.5393,
      "us-west1": 0.4736,
      "europe": 0.5208,
      "europe-west1": 0.5208,
      "europe-west2": 0.6092,
      "europe-west3": 0.6092,
      "asia": 0.5480,
      "asia-east": 0.5480,
      "asia-northeast": 0.6068,
      "asia-southeast": 0.5840,
      "australia-southeast1": 0.6716,
      "australia": 0.6716,
      "southamerica-east1": 0.7514,
      "asia-south1": 0.5683,
      "cores": "8",
      "memory": "52",
      "gceu": 22,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHMEM-16": {
      "us": 0.9472,
      "us-central1": 0.9472,
      "us-east1": 0.9472,
      "us-east4": 1.0786,
      "us-west1": 0.9472,
      "europe": 1.0416,
      "europe-west1": 1.0416,
      "europe-west2": 1.2184,
      "europe-west3": 1.2184,
      "asia": 1.0960,
      "asia-east": 1.0960,
      "asia-northeast": 1.2136,
      "asia-southeast": 1.1680,
      "australia-southeast1":1.3431,
      "australia":1.3431,
      "southamerica-east1": 1.5029,
      "asia-south1": 1.1366,
      "cores": "16",
      "memory": "104",
      "gceu": 44,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHMEM-32": {
      "us": 1.8944,
      "us-central1": 1.8944,
      "us-east1": 1.8944,
      "us-east4": 2.1571,
      "us-west1": 1.8944,
      "europe": 2.0832,
      "europe-west1": 2.0832,
      "europe-west2": 2.4368,
      "europe-west3": 2.4368,
      "asia": 2.1920,
      "asia-east": 2.1920,
      "asia-northeast": 2.4272,
      "asia-southeast": 2.3360,
      "australia-southeast1":2.6862,
      "australia":2.6862,
      "southamerica-east1": 3.0057,
      "asia-south1": 2.2732,
      "cores": "32",
      "memory": "208",
      "gceu": 88,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHMEM-64": {
      "us": 3.7888,
      "us-central1": 3.7888,
      "us-east1": 3.7888,
      "us-east4": 4.3142,
      "us-west1": 3.7888,
      "europe": 4.1664,
      "europe-west1": 4.1664,
      "europe-west2": 4.8736,
      "europe-west3": 4.8736,
      "asia": 4.3840,
      "asia-east": 4.3840,
      "asia-northeast": 4.8544,
      "asia-southeast": 4.6720,
      "australia-southeast1":5.3725,
      "australia":5.3725,
      "southamerica-east1": 6.0114,
      "asia-south1": 4.5465,
      "cores": "64",
      "memory": "416",
      "gceu": 176,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHMEM-96": {
      "us": 5.6832,
      "us-central1": 5.6832,
      "us-east1": 0,
      "us-east4": 0,
      "us-west1": 5.6832,
      "europe": 6.2496,
      "europe-west1": 6.2496,
      "europe-west2": 0,
      "europe-west3": 0,
      "asia": 6.5760,
      "asia-east": 6.5760,
      "asia-northeast": 0,
      "asia-southeast": 0,
      "australia-southeast1": 0,
      "australia": 0,
      "southamerica-east1": 0,
      "asia-south1": 0,
      "cores": "96",
      "memory": "624",
      "gceu": 264,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHCPU-2": {
      "us": 0.0709,
      "us-central1": 0.0709,
      "us-east1": 0.0709,
      "us-east4": 0.0813,
      "us-west1": 0.0709,
      "europe": 0.0780,
      "europe-west1": 0.0780,
      "europe-west2": 0.0912,
      "europe-west3": 0.0912,
      "asia": 0.0821,
      "asia-east": 0.0821,
      "asia-northeast": 0.0910,
      "asia-southeast": 0.0874,
      "australia-southeast1": 0.1006,
      "australia": 0.1006,
      "southamerica-east1": 0.1125,
      "asia-south1": 0.0851,
      "cores": "2",
      "memory": "1.8",
      "gceu": 5.5,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHCPU-4": {
      "us": 0.1418,
      "us-central1": 0.1418,
      "us-east1": 0.1418,
      "us-east4": 0.1626,
      "us-west1": 0.1418,
      "europe": 0.1560,
      "europe-west1": 0.1560,
      "europe-west2": 0.1824,
      "europe-west3": 0.1824,
      "asia": 0.1642,
      "asia-east": 0.1642,
      "asia-northeast": 0.1820,
      "asia-southeast": 0.1748,
      "australia-southeast1": 0.2012,
      "australia": 0.2012,
      "southamerica-east1": 0.2250,
      "asia-south1": 0.1702,
      "cores": "4",
      "memory": "3.6",
      "gceu": 11,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHCPU-8": {
      "us": 0.2836,
      "us-central1": 0.2836,
      "us-east1": 0.2836,
      "us-east4": 0.3253,
      "us-west1": 0.2836,
      "europe": 0.3120,
      "europe-west1": 0.3120,
      "europe-west2": 0.3648,
      "europe-west3": 0.3648,
      "asia": 0.3284,
      "asia-east": 0.3284,
      "asia-northeast": 0.3640,
      "asia-southeast": 0.3496,
      "australia-southeast1": 0.4023,
      "australia": 0.4023,
      "southamerica-east1": 0.4500,
      "asia-south1": 0.3404,
      "cores": "8",
      "memory": "7.2",
      "gceu": 22,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHCPU-16": {
      "us": 0.5672,
      "us-central1": 0.5672,
      "us-east1": 0.5672,
      "us-east4": 0.6506,
      "us-west1": 0.5672,
      "europe": 0.6240,
      "europe-west1": 0.6240,
      "europe-west2": 0.7296,
      "europe-west3": 0.7296,
      "asia": 0.6568,
      "asia-east": 0.6568,
      "asia-northeast": 0.7280,
      "asia-southeast": 0.6992,
      "australia-southeast1": 0.8046,
      "australia": 0.8046,
      "southamerica-east1": 0.8999,
      "asia-south1": 0.6807,
      "cores": "16",
      "memory": "14.40",
      "gceu": 44,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHCPU-32": {
      "us": 1.1344,
      "us-central1": 1.1344,
      "us-east1": 1.1344,
      "us-east4": 1.3011,
      "us-west1": 1.1344,
      "europe": 1.2480,
      "europe-west1": 1.2480,
      "europe-west2": 1.4592,
      "europe-west3": 1.4592,
      "asia": 1.3136,
      "asia-east": 1.3136,
      "asia-northeast": 1.4560,
      "asia-southeast": 1.3984,
      "australia-southeast1":1.6092,
      "australia":1.6092,
      "southamerica-east1": 1.7999,
      "asia-south1": 1.3615,
      "cores": "32",
      "memory": "28.80",
      "gceu": 88,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHCPU-64": {
      "us": 2.2688,
      "us-central1": 2.2688,
      "us-east1": 2.2688,
      "us-east4": 2.6022,
      "us-west1": 2.2688,
      "europe": 2.4960,
      "europe-west1": 2.4960,
      "europe-west2": 2.9184,
      "europe-west3": 2.9184,
      "asia": 2.6272,
      "asia-east": 2.6272,
      "asia-northeast": 2.9120,
      "asia-southeast": 2.7968,
      "australia-southeast1":3.2185,
      "australia":3.2185,
      "southamerica-east1": 3.5998,
      "asia-south1": 2.7229,
      "cores": "64",
      "memory": "57.6",
      "gceu": 176,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHCPU-96": {
      "us": 3.4032,
      "us-central1": 3.4032,
      "us-east1": 0,
      "us-east4": 0,
      "us-west1": 3.4032,
      "europe": 3.7440,
      "europe-west1": 3.7440,
      "europe-west2": 0,
      "europe-west3": 0,
      "asia": 3.9408,
      "asia-east": 3.9408,
      "asia-northeast": 0,
      "asia-southeast": 0,
      "australia-southeast1": 0,
      "australia": 0,
      "southamerica-east1": 0,
      "asia-south1": 0,
      "cores": "96",
      "memory": "86.4",
      "gceu": 264,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-MEGAMEM-96": {
      "us": 10.67,
      "us-central1": 10.67,
      "us-east1": 0,
      "us-east4": 0,
      "us-west1": 10.67,
      "europe": 11.74,
      "europe-west1": 11.74,
      "europe-west2": 0,
      "europe-west3": 0,
      "asia": 12.36,
      "asia-east": 12.36,
      "asia-northeast": 0,
      "asia-southeast": 0,
      "australia-southeast1": 0,
      "australia": 0,
      "southamerica-east1": 0,
      "asia-south1": 0,
      "cores": "96",
      "memory": "1440",
      "gceu": 264,
      "maxNumberOfPd": 16,
      "maxPdSize": 64,
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-F1-MICRO-PREEMPTIBLE": {
      "us": 0.0035,
      "us-central1": 0.0035,
      "us-east1": 0.0035,
      "us-east4": 0.00375,
      "us-west1": 0.0035,
      "europe": 0.0039,
      "europe-west1": 0.0039,
      "europe-west2": 0.00420,
      "europe-west3": 0.00420,
      "asia": 0.0039,
      "asia-east": 0.0039,
      "asia-northeast": 0.005,
      "asia-southeast": 0.0040,
      "australia-southeast1": 0.00460,
      "australia": 0.00460,
      "southamerica-east1": 0.00520,
      "asia-south1": 0.00420,
      "cores": "shared",
      "memory": "0.6",
      "ssd": [0]
    },
    "CP-COMPUTEENGINE-VMIMAGE-G1-SMALL-PREEMPTIBLE": {
      "us": 0.007,
      "us-central1": 0.007,
      "us-east1": 0.007,
      "us-east4": 0.00749,
      "us-west1": 0.007,
      "europe": 0.0077,
      "europe-west1": 0.0077,
      "europe-west2": 0.00840,
      "europe-west3": 0.00840,
      "asia": 0.0077,
      "asia-east": 0.0077,
      "asia-northeast": 0.0100,
      "asia-southeast": 0.0081,
      "australia-southeast1": 0.00930,
      "australia": 0.00930,
      "southamerica-east1": 0.01040,
      "asia-south1": 0.00840,
      "cores": "shared",
      "memory": "1.7",
      "ssd": [0]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-1-PREEMPTIBLE": {
      "us": 0.01,
      "us-central1": 0.01,
      "us-east1": 0.01,
      "us-east4": 0.01070,
      "us-west1": 0.01,
      "europe": 0.011,
      "europe-west1": 0.011,
      "europe-west2": 0.01230,
      "europe-west3": 0.01230,
      "asia": 0.011,
      "asia-east": 0.011,
      "asia-northeast": 0.01325,
      "asia-southeast": 0.01180,
      "australia-southeast1": 0.01349,
      "australia": 0.01349,
      "southamerica-east1": 0.01510,
      "asia-south1": 0.01203,
      "cores": "1",
      "memory": "3.75",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-2-PREEMPTIBLE": {
      "us": 0.02,
      "us-central1": 0.02,
      "us-east1": 0.02,
      "us-east4": 0.02140,
      "us-west1": 0.02,
      "europe": 0.022,
      "europe-west1": 0.022,
      "europe-west2": 0.02460,
      "europe-west3": 0.02460,
      "asia": 0.022,
      "asia-east": 0.022,
      "asia-northeast": 0.0265,
      "asia-southeast": 0.0236,
      "australia-southeast1": 0.02698,
      "australia": 0.02698,
      "southamerica-east1": 0.03020,
      "asia-south1": 0.02405,
      "cores": "2",
      "memory": "7.5",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-4-PREEMPTIBLE": {
      "us": 0.04,
      "us-central1": 0.04,
      "us-east1": 0.04,
      "us-east4": 0.04280,
      "us-west1": 0.04,
      "europe": 0.044,
      "europe-west1": 0.044,
      "europe-west2": 0.04920,
      "europe-west3": 0.04920,
      "asia": 0.044,
      "asia-east": 0.044,
      "asia-northeast": 0.053,
      "asia-southeast": 0.0472,
      "australia-southeast1": 0.05397,
      "australia": 0.05397,
      "southamerica-east1": 0.06040,
      "asia-south1": 0.04811,
      "cores": "4",
      "memory": "15",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-8-PREEMPTIBLE": {
      "us": 0.08,
      "us-central1": 0.08,
      "us-east1": 0.08,
      "us-east4": 0.08560,
      "us-west1": 0.08,
      "europe": 0.088,
      "europe-west1": 0.088,
      "europe-west2": 0.09840,
      "europe-west3": 0.09840,
      "asia": 0.088,
      "asia-east": 0.088,
      "asia-northeast": 0.106,
      "asia-southeast": 0.0944,
      "australia-southeast1": 0.10793,
      "australia": 0.10793,
      "southamerica-east1": 0.12080,
      "asia-south1": 0.09622,
      "cores": "8",
      "memory": "30",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-16-PREEMPTIBLE": {
      "us": 0.16,
      "us-central1": 0.16,
      "us-east1": 0.16,
      "us-east4": 0.17120,
      "us-west1": 0.16,
      "europe": 0.176,
      "europe-west1": 0.176,
      "europe-west2": 0.19680,
      "europe-west3": 0.19680,
      "asia": 0.176,
      "asia-east": 0.176,
      "asia-northeast": 0.212,
      "asia-southeast": 0.1888,
      "australia-southeast1": 0.21586,
      "australia": 0.21586,
      "southamerica-east1": 0.24160,
      "asia-south1": 0.19243,
      "cores": "16",
      "memory": "60",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-32-PREEMPTIBLE": {
      "us": 0.32,
      "us-central1": 0.32,
      "us-east1": 0.32,
      "us-east4": 0.34240,
      "us-west1": 0.32,
      "europe": 0.352,
      "europe-west1": 0.352,
      "europe-west2": 0.39360,
      "europe-west3": 0.39360,
      "asia": 0.352,
      "asia-east": 0.352,
      "asia-northeast": 0.424,
      "asia-southeast": 0.3776,
      "australia-southeast1": 0.43172,
      "australia": 0.43172,
      "southamerica-east1": 0.48320,
      "asia-south1": 0.38486,
      "cores": "32",
      "memory": "120",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-64-PREEMPTIBLE": {
      "us": 0.640,
      "us-central1": 0.640,
      "us-east1": 0.640,
      "us-east4": 0.6848,
      "us-west1": 0.640,
      "europe": 0.704,
      "europe-west1": 0.704,
      "europe-west2": 0.78720,
      "europe-west3": 0.78720,
      "asia": 0.704,
      "asia-east": 0.704,
      "asia-northeast": 0.8480,
      "asia-southeast": 0.7552,
      "australia-southeast1": 0.86344,
      "australia": 0.86344,
      "southamerica-east1": 0.96640,
      "asia-south1": 0.76973,
      "cores": "64",
      "memory": "240",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-STANDARD-96-PREEMPTIBLE": {
      "us": 0.9600,
      "us-central1": 0.9600,
      "us-east1": 0,
      "us-east4": 0,
      "us-west1": 0.9600,
      "europe": 1.0560,
      "europe-west1": 1.0560,
      "europe-west2": 0,
      "europe-west3": 0,
      "asia": 1.0560,
      "asia-east": 1.0560,
      "asia-northeast": 0,
      "asia-southeast": 0,
      "australia-southeast1": 0,
      "australia": 0,
      "southamerica-east1": 0,
      "asia-south1": 0,
      "cores": "96",
      "memory": "360",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHMEM-2-PREEMPTIBLE": {
      "us": 0.025,
      "us-central1": 0.025,
      "us-east1": 0.025,
      "us-east4": 0.02675,
      "us-west1": 0.025,
      "europe": 0.0275,
      "europe-west1": 0.0275,
      "europe-west2": 0.03050,
      "europe-west3": 0.03050,
      "asia": 0.0275,
      "asia-east": 0.0275,
      "asia-northeast": 0.033,
      "asia-southeast": 0.0292,
      "australia-southeast1": 0.03360,
      "australia": 0.03360,
      "southamerica-east1": 0.03757,
      "asia-south1": 0.02997,
      "cores": "2",
      "memory": "13",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHMEM-4-PREEMPTIBLE": {
      "us": 0.05,
      "us-central1": 0.05,
      "us-east1": 0.05,
      "us-east4": 0.05350,
      "us-west1": 0.05,
      "europe": 0.055,
      "europe-west1": 0.055,
      "europe-west2": 0.06100,
      "europe-west3": 0.06100,
      "asia": 0.055,
      "asia-east": 0.055,
      "asia-northeast": 0.066,
      "asia-southeast": 0.0584,
      "australia-southeast1": 0.06720,
      "australia": 0.06720,
      "southamerica-east1": 0.07515,
      "asia-south1": 0.05994,
      "cores": "4",
      "memory": "26",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHMEM-8-PREEMPTIBLE": {
      "us": 0.1,
      "us-central1": 0.1,
      "us-east1": 0.1,
      "us-east4": 0.10700,
      "us-west1": 0.1,
      "europe": 0.11,
      "europe-west1": 0.11,
      "europe-west2": 0.12200,
      "europe-west3": 0.12200,
      "asia": 0.11,
      "asia-east": 0.11,
      "asia-northeast": 0.132,
      "asia-southeast": 0.1168,
      "australia-southeast1": 0.13440,
      "australia": 0.13440,
      "southamerica-east1": 0.15030,
      "asia-south1": 0.11989,
      "cores": "8",
      "memory": "52",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHMEM-16-PREEMPTIBLE": {
      "us": 0.2,
      "us-central1": 0.2,
      "us-east1": 0.2,
      "us-east4": 0.21400,
      "us-west1": 0.2,
      "europe": 0.22,
      "europe-west1": 0.22,
      "europe-west2": 0.24400,
      "europe-west3": 0.24400,
      "asia": 0.22,
      "asia-east": 0.22,
      "asia-northeast": 0.264,
      "asia-southeast": 0.2336,
      "australia-southeast1": 0.26879,
      "australia": 0.26879,
      "southamerica-east1": 0.30059,
      "asia-south1": 0.23978,
      "cores": "16",
      "memory": "104",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHMEM-32-PREEMPTIBLE": {
      "us": 0.4,
      "us-central1": 0.4,
      "us-east1": 0.4,
      "us-east4": 0.42800,
      "us-west1": 0.4,
      "europe": 0.44,
      "europe-west1": 0.44,
      "europe-west2": 0.48800,
      "europe-west3": 0.48800,
      "asia": 0.44,
      "asia-east": 0.44,
      "asia-northeast": 0.528,
      "asia-southeast": 0.4672,
      "australia-southeast1": 0.53758,
      "australia": 0.53758,
      "southamerica-east1": 0.60118,
      "asia-south1": 0.47955,
      "cores": "32",
      "memory": "208",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHMEM-64-PREEMPTIBLE": {
      "us": 0.800,
      "us-central1": 0.800,
      "us-east1": 0.800,
      "us-east4": 0.8560,
      "us-west1": 0.800,
      "europe": 0.880,
      "europe-west1": 0.880,
      "europe-west2": 0.97600,
      "europe-west3": 0.97600,
      "asia": 0.880,
      "asia-east": 0.880,
      "asia-northeast": 1.0560,
      "asia-southeast": 0.9344,
      "australia-southeast1":1.07517,
      "australia":1.07517,
      "southamerica-east1": 1.20237,
      "asia-south1": 0.95910,
      "cores": "64",
      "memory": "416",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHMEM-96-PREEMPTIBLE": {
      "us": 1.2000,
      "us-central1": 1.2000,
      "us-east1": 0,
      "us-east4": 0,
      "us-west1": 1.2000,
      "europe": 1.3200,
      "europe-west1": 1.3200,
      "europe-west2": 0,
      "europe-west3": 0,
      "asia": 1.3200,
      "asia-east": 1.3200,
      "asia-northeast": 0,
      "asia-southeast": 0,
      "australia-southeast1": 0,
      "australia": 0,
      "southamerica-east1": 0,
      "asia-south1": 0,
      "cores": "96",
      "memory": "624",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHCPU-2-PREEMPTIBLE": {
      "us": 0.015,
      "us-central1": 0.015,
      "us-east1": 0.015,
      "us-east4": 0.01605,
      "us-west1": 0.015,
      "europe": 0.0165,
      "europe-west1": 0.0165,
      "europe-west2": 0.01830,
      "europe-west3": 0.01830,
      "asia": 0.0165,
      "asia-east": 0.0165,
      "asia-northeast": 0.0198,
      "asia-southeast": 0.0175,
      "australia-southeast1": 0.02013,
      "australia": 0.02013,
      "southamerica-east1": 0.02250,
      "asia-south1": 0.01792,
      "cores": "2",
      "memory": "1.8",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHCPU-4-PREEMPTIBLE": {
      "us": 0.03,
      "us-central1": 0.03,
      "us-east1": 0.03,
      "us-east4": 0.03210,
      "us-west1": 0.03,
      "europe": 0.033,
      "europe-west1": 0.033,
      "europe-west2": 0.03660,
      "europe-west3": 0.03660,
      "asia": 0.033,
      "asia-east": 0.033,
      "asia-northeast": 0.0396,
      "asia-southeast": 0.0350,
      "australia-southeast1": 0.04025,
      "australia": 0.04025,
      "southamerica-east1": 0.04500,
      "asia-south1": 0.03584,
      "cores": "4",
      "memory": "3.6",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHCPU-8-PREEMPTIBLE": {
      "us": 0.06,
      "us-central1": 0.06,
      "us-east1": 0.06,
      "us-east4": 0.06420,
      "us-west1": 0.06,
      "europe": 0.066,
      "europe-west1": 0.066,
      "europe-west2": 0.07320,
      "europe-west3": 0.07320,
      "asia": 0.066,
      "asia-east": 0.066,
      "asia-northeast": 0.0792,
      "asia-southeast": 0.0700,
      "australia-southeast1": 0.08050,
      "australia": 0.08050,
      "southamerica-east1": 0.09000,
      "asia-south1": 0.07168,
      "cores": "8",
      "memory": "7.2",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHCPU-16-PREEMPTIBLE": {
      "us": 0.12,
      "us-central1": 0.12,
      "us-east1": 0.12,
      "us-east4": 0.12840,
      "us-west1": 0.12,
      "europe": 0.132,
      "europe-west1": 0.132,
      "europe-west2": 0.14640,
      "europe-west3": 0.14640,
      "asia": 0.132,
      "asia-east": 0.132,
      "asia-northeast": 0.1584,
      "asia-southeast": 0.1400,
      "australia-southeast1": 0.16100,
      "australia": 0.16100,
      "southamerica-east1": 0.17999,
      "asia-south1": 0.14337,
      "cores": "16",
      "memory": "14.40",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHCPU-32-PREEMPTIBLE": {
      "us": 0.24,
      "us-central1": 0.24,
      "us-east1": 0.24,
      "us-east4": 0.25680,
      "us-west1": 0.24,
      "europe": 0.264,
      "europe-west1": 0.264,
      "europe-west2": 0.29280,
      "europe-west3": 0.29280,
      "asia": 0.264,
      "asia-east": 0.264,
      "asia-northeast": 0.3168,
      "asia-southeast": 0.2800,
      "australia-southeast1": 0.32201,
      "australia": 0.32201,
      "southamerica-east1": 0.35998,
      "asia-south1": 0.28673,
      "cores": "32",
      "memory": "28.80",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHCPU-64-PREEMPTIBLE": {
      "us": 0.480,
      "us-central1": 0.480,
      "us-east1": 0.480,
      "us-east4": 0.5136,
      "us-west1": 0.480,
      "europe": 0.528,
      "europe-west1": 0.528,
      "europe-west2": 0.58560,
      "europe-west3": 0.58560,
      "asia": 0.528,
      "asia-east": 0.528,
      "asia-northeast": 0.6336,
      "asia-southeast": 0.5600,
      "australia-southeast1": 0.64401,
      "australia": 0.64401,
      "southamerica-east1": 0.71996,
      "asia-south1": 0.57347,
      "cores": "64",
      "memory": "57.6",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-HIGHCPU-96-PREEMPTIBLE": {
      "us": 0.7200,
      "us-central1": 0.7200,
      "us-east1": 0,
      "us-east4": 0,
      "us-west1": 0.7200,
      "europe": 0.7920,
      "europe-west1": 0.7920,
      "europe-west2": 0,
      "europe-west3": 0,
      "asia": 0.7920,
      "asia-east": 0.7920,
      "asia-northeast": 0,
      "asia-southeast": 0,
      "australia-southeast1": 0,
      "australia": 0,
      "southamerica-east1": 0,
      "asia-south1": 0,
      "cores": "96",
      "memory": "86.4",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-VMIMAGE-N1-MEGAMEM-96-PREEMPTIBLE": {
      "us": 2.26,
      "us-central1": 2.26,
      "us-east1": 0,
      "us-east4": 0,
      "us-west1": 2.26,
      "europe": 2.47,
      "europe-west1": 2.47,
      "europe-west2": 0,
      "europe-west3": 0,
      "asia": 2.47,
      "asia-east": 2.47,
      "asia-northeast": 0,
      "asia-southeast": 0,
      "australia-southeast1": 0,
      "australia": 0,
      "southamerica-east1": 0,
      "asia-south1": 0,
      "cores": "96",
      "memory": "1440",
      "ssd": [0, 1, 2, 3, 4, 5, 6, 7, 8]
    },
    "CP-COMPUTEENGINE-LOCAL-SSD": {
      "us": 0.00010959,
      "us-central1": 0.00010959,
      "us-east1": 0.00010959,
      "us-east4": 0.00012055,
      "us-west1": 0.00010959,
      "europe": 0.00010959,
      "europe-west1": 0.00010959,
      "europe-west2": 0.00013151,
      "europe-west3": 0.00013151,
      "asia": 0.00010959,
      "asia-east": 0.00010959,
      "asia-northeast": 0.00014247,
      "asia-southeast": 0.00012055,
      "australia-southeast1": 0.00014795,
      "australia": 0.00014795,
      "southamerica-east1": 0.000164385,
      "asia-south1": 0.00013151
    },
    "CP-COMPUTEENGINE-LOCAL-SSD-PREEMPTIBLE": {
      "us": 0.000065754,
      "us-central1": 0.000065754,
      "us-east1": 0.000065754,
      "us-east4": 0.00009589,
      "us-west1": 0.000065754,
      "europe": 0.000065754,
      "europe-west1": 0.000065754,
      "europe-west2": 0.000079453,
      "europe-west3": 0.000079453,
      "asia": 0.000065754,
      "asia-east": 0.000065754,
      "asia-northeast": 0.000084932,
      "asia-southeast": 0.00009589,
      "australia-southeast1": 0.0000890411,
      "australia": 0.0000890411,
      "southamerica-east1": 0.000098631,
      "asia-south1": 0.000079453
    },
    "CP-COMPUTEENGINE-OS": {
      "win": {
        "low": 0.02,
        "high": 0.04,
        "cores": "shared",
        "percore": true
      },
      "windows-server-core": null,
      "rhel": {
        "low": 0.06,
        "high": 0.13,
        "cores": "4",
        "percore": false
      },
      "suse": {
        "low": 0.02,
        "high": 0.11,
        "cores": "shared",
        "percore": false
      },
      "suse-sap": {
        "low": 0.75,
        "high": 0.75,
        "cores": "shared",
        "percore": false
      },
      "sql-standard": {
        "low": 0.1645,
        "high": 0.1645,
        "cores": "4",
        "percore": true
      },
      "sql-web": {
        "low": 0.011,
        "high": 0.011,
        "cores": "4",
        "percore": true
      },
      "sql-enterprise": {
        "low": 0.399,
        "high": 0.399,
        "cores": "4",
        "percore": true
      }
    },
    "CP-COMPUTEENGINE-STORAGE-PD-CAPACITY": {
      "us": 0.04,
      "us-central1": 0.04,
      "us-east1": 0.04,
      "us-east4": 0.044,
      "us-west1": 0.04,
      "europe": 0.04,
      "europe-west1": 0.04,
      "europe-west2": 0.048,
      "europe-west3": 0.048,
      "asia-east": 0.04,
      "asia-northeast": 0.052,
      "asia-southeast": 0.044,
      "australia-southeast1": 0.054,
      "australia": 0.054,
      "southamerica-east1": 0.060,
      "asia-south1": 0.048
    },
    "CP-COMPUTEENGINE-STORAGE-PD-SSD": {
      "us": 0.17,
      "us-central1": 0.17,
      "us-east1": 0.17,
      "us-east4": 0.187,
      "us-west1": 0.17,
      "europe": 0.17,
      "europe-west1": 0.17,
      "europe-west2": 0.204,
      "europe-west3": 0.204,
      "asia-east": 0.17,
      "asia-northeast": 0.221,
      "asia-southeast": 0.187,
      "australia-southeast1": 0.230,
      "australia": 0.230,
      "southamerica-east1": 0.255,
      "asia-south1": 0.204
    },
    "CP-COMPUTEENGINE-PD-IO-REQUEST": {
      "us": 0.0
    },
    "CP-COMPUTEENGINE-STORAGE-PD-SNAPSHOT": {
      "us": 0.026,
      "us-central1": 0.026,
      "us-east1": 0.026,
      "us-east4": 0.029,
      "us-west1": 0.026,
      "europe": 0.026,
      "europe-west1": 0.026,
      "europe-west2": 0.031,
      "europe-west3": 0.031,
      "asia-east": 0.026,
      "asia-northeast": 0.034,
      "asia-southeast": 0.039,
      "australia-southeast1": 0.035,
      "australia": 0.035,
      "southamerica-east1": 0.039,
      "asia-south1": 0.031
    },
    "CP-BIGSTORE-CLASS-A-REQUEST": {
      "us": 0.05,
      "us-central1": 0.05,
      "us-east1": 0.05,
      "us-east4": 0.05,
      "us-west1": 0.05,
      "europe": 0.05,
      "europe-west1": 0.05,
      "europe-west2": 0.05,
      "europe-west3": 0.05,
      "asia-east": 0.05,
      "asia-northeast": 0.05,
      "asia-southeast": 0.05,
      "australia-southeast1": 0.5,
      "australia": 0.05,
      "southamerica-east1": 0.5,
      "asia-south1": 0.5
    },
    "CP-BIGSTORE-CLASS-B-REQUEST": {
      "us": 0.004,
      "us-central1": 0.004,
      "us-east1": 0.004,
      "us-east4": 0.004,
      "us-west1": 0.004,
      "europe": 0.004,
      "europe-west1": 0.004,
      "europe-west2": 0.004,
      "europe-west3": 0.004,
      "asia-east": 0.004,
      "asia-northeast": 0.004,
      "asia-southeast": 0.004,
      "australia-southeast1": 0.004,
      "australia": 0.004,
      "asia-south1": 0.004
    },
    "CP-CLOUDSQL-PERUSE-D0": {
      "us": 0.025
    },
    "CP-CLOUDSQL-PERUSE-D1": {
      "us": 0.10
    },
    "CP-CLOUDSQL-PERUSE-D2": {
      "us": 0.19
    },
    "CP-CLOUDSQL-PERUSE-D4": {
      "us": 0.29
    },
    "CP-CLOUDSQL-PERUSE-D8": {
      "us": 0.58
    },
    "CP-CLOUDSQL-PERUSE-D16": {
      "us": 1.16
    },
    "CP-CLOUDSQL-PERUSE-D32": {
      "us": 2.31
    },
    "CP-CLOUDSQL-PACKAGE-D0": {
      "us": 0.360
    },
    "CP-CLOUDSQL-PACKAGE-D1": {
      "us": 1.46
    },
    "CP-CLOUDSQL-PACKAGE-D2": {
      "us": 2.93
    },
    "CP-CLOUDSQL-PACKAGE-D4": {
      "us": 4.4
    },
    "CP-CLOUDSQL-PACKAGE-D8": {
      "us": 8.78
    },
    "CP-CLOUDSQL-PACKAGE-D16": {
      "us": 17.57
    },
    "CP-CLOUDSQL-PACKAGE-D32": {
      "us": 35.13
    },
    "CP-CLOUDSQL-STORAGE": {
      "us": 0.24
    },
    "CP-CLOUDSQL-TRAFFIC": {
      "us": 0.12
    },
    "CP-CLOUDSQL-IO": {
      "us": 0.10
    },
    "CP-BIGSTORE-STORAGE": {
      "us": 0.026
    },
    "CP-BIGSTORE-STORAGE-DRA": {
      "us": 0.02
    },
    "CP-NEARLINE-STORAGE": {
      "us": 0.01,
      "us-central1": 0.01,
      "us-east1": 0.01,
      "us-east4": 0.016,
      "us-west1": 0.01,
      "europe": 0.01,
      "europe-west1": 0.01,
      "europe-west2": 0.016,
      "europe-west3": 0.016,
      "asia-east": 0.01,
      "asia-northeast": 0.016
    },
    "CP-NEARLINE-RESTORE-SIZE": {
      "us": 0.01
    },
    "FORWARDING_RULE_CHARGE_BASE": {
      "us": 0.025,
      "us-central1": 0.025,
      "us-east1": 0.025,
      "us-east4": 0.028,
      "us-west1": 0.025,
      "europe": 0.025,
      "europe-west1": 0.025,
      "europe-west2": 0.030,
      "europe-west3": 0.030,
      "asia-east": 0.025,
      "asia-northeast": 0.038,
      "asia-southeast": 0.028,
      "australia-southeast1": 0.034,
      "australia": 0.034,
      "southamerica-east1": 0.038,
      "asia-south1": 0.030,
      "fixed": true
    },
    "FORWARDING_RULE_CHARGE_EXTRA": {
      "us": 0.010,
      "us-central1": 0.010,
      "us-east1": 0.010,
      "us-east4": 0.11,
      "us-west1": 0.010,
      "europe": 0.010,
      "europe-west1": 0.010,
      "europe-west2": 0.012,
      "europe-west3": 0.012,
      "asia-east": 0.010,
      "asia-northeast": 0.011,
      "asia-southeast": 0.011,
      "australia-southeast1": 0.014,
      "australia": 0.014,
      "southamerica-east1": 0.015,
      "asia-south1": 0.012
    },
    "NETWORK_LOAD_BALANCED_INGRESS": {
      "us": 0.008,
      "us-central1": 0.008,
      "us-east1": 0.008,
      "us-east4": 0.009,
      "us-west1": 0.008,
      "europe": 0.008,
      "europe-west1": 0.008,
      "europe-west2": 0.010,
      "europe-west3": 0.010,
      "asia-east": 0.008,
      "asia-northeast": 0.012,
      "asia-southeast": 0.009,
      "australia-southeast1": 0.011,
      "australia": 0.011,
      "southamerica-east1": 0.012,
      "asia-south1": 0.010
    },
    "CP-COMPUTEENGINE-INTERNET-EGRESS-NA-NA": {
      "tiers": {
        "1024": 0.12,
        "10240": 0.11,
        "92160": 0.08
      }
    },
    "CP-COMPUTEENGINE-INTERNET-EGRESS-APAC-APAC": {
      "tiers": {
        "1024": 0.12,
        "10240": 0.11,
        "92160": 0.08
      }
    },
    "CP-COMPUTEENGINE-INTERNET-EGRESS-AU-AU": {
      "tiers": {
        "1024": 0.19,
        "10240": 0.18,
        "92160": 0.15
      }
    },
    "CP-COMPUTEENGINE-INTERNET-EGRESS-CN-CN": {
      "tiers": {
        "1024": 0.23,
        "10240": 0.22,
        "92160": 0.20
      }
    },
    "CP-COMPUTEENGINE-INTERCONNECT-US-US": {
      "us": 0.04
    },
    "CP-COMPUTEENGINE-INTERCONNECT-EU-EU": {
      "us": 0.05
    },
    "CP-COMPUTEENGINE-INTERCONNECT-APAC-APAC": {
      "us": 0.06
    },
    "CP-COMPUTEENGINE-INTERNET-EGRESS-ZONE": {
      "us": 0.01
    },
    "CP-COMPUTEENGINE-INTERNET-EGRESS-REGION": {
      "us": 0.01
    },
    "CP-APP-ENGINE-INSTANCES": {
      "us": 0.05,
      "freequota": {
        "quantity": 851.2
      }
    },
    "CP-APP-ENGINE-OUTGOING-TRAFFIC": {
      "us": 0.12,
      "freequota": {
        "quantity": 1
      }
    },
    "CP-APP-ENGINE-CLOUD-STORAGE": {
      "us": 0.026,
      "freequota": {
        "quantity": 5
      }
    },
    "CP-APP-ENGINE-MEMCACHE": {
      "us": 0.06
    },
    "CP-APP-ENGINE-SEARCH": {
      "us": 0.00005,
      "freequota": {
        "quantity": 100
      }
    },
    "CP-APP-ENGINE-INDEXING-DOCUMENTS": {
      "us": 2.0,
      "freequota": {
        "quantity": 0.01
      }
    },
    "CP-APP-ENGINE-DOCUMENT-STORAGE": {
      "us": 0.18,
      "freequota": {
        "quantity": 0.25
      }
    },
    "CP-APP-ENGINE-LOGS-API": {
      "us": 0.12,
      "freequota": {
        "quantity": 0.1
      }
    },
    "CP-APP-ENGINE-TASK-QUEUE": {
      "us": 0.026,
      "freequota": {
        "quantity": 5
      }
    },
    "CP-APP-ENGINE-LOGS-STORAGE": {
      "us": 0.026,
      "freequota": {
        "quantity": 1
      }
    },
    "CP-APP-ENGINE-SSL-VIRTUAL-IP": {
      "us": 39
    },
    "CP-CLOUD-DATASTORE-INSTANCES": {
      "us": 0.18,
      "freequota": {
        "quantity": 30.4
      }
    },
    "CP-CLOUD-DATASTORE-WRITE-OP": {
      "us": 0.0000006,
      "freequota": {
        "quantity": 1520000
      }
    },
    "CP-CLOUD-DATASTORE-READ-OP": {
      "us": 0.0000006,
      "freequota": {
        "quantity": 1520000
      }
    },
    "CP-CLOUD-DATASTORE-ENTITY-READ": {
      "us": 0.0000006,
      "freequota": {
        "quantity": 1520000
      }
    },
    "CP-CLOUD-DATASTORE-ENTITY-WRITE": {
      "us": 0.0000018,
      "freequota": {
        "quantity": 608000
      }
    },
    "CP-CLOUD-DATASTORE-ENTITY-DELETE": {
      "us": 0.0000002,
      "freequota": {
        "quantity": 608000
      }
    },
    "CP-BIGQUERY-GENERAL": {
      "storage": {
        "us": 0.02
      },
      "interactiveQueries": {
        "us": 5,
        "freequota": {
          "quantity": 1
        }
      },
      "streamingInserts": {
        "us": 0.00005
      }
    },
    "CP-CLOUD-DNS-ZONES": {
      "tiers": {
        "25": 0.2,
        "10000": 0.1,
        "100000": 0.03
      }
    },
    "CP-CLOUD-DNS-QUERIES": {
      "tiers": {
        "1000000000": 0.0000004,
        "10000000000": 0.0000002
      }
    },
    "CP-TRANSLATE-API-TRANSLATION": {
      "tiers": {
        "1500000000": 0.00002,
        "3000000000": 0.000015
      }
    },
    "CP-TRANSLATE-API-DETECTION": {
      "tiers": {
        "1500000000": 0.00002,
        "3000000000": 0.000015
      }
    },
    "CP-PREDICTION-PREDICTION": {
      "tiers": {
        "10000": 0,
        "100000": 0.0005
      }
    },
    "CP-PREDICTION-BULK-TRAINING": {
      "us": 0.002
    },
    "CP-PREDICTION-STREAMING-TRAINING": {
      "tiers": {
        "10000": 0,
        "100000": 0.00005
      }
    },
    "CP-GENOMICS-STORAGE": {
      "us": 0.022
    },
    "CP-GENOMICS-QUERIES": {
      "us": 1.0
    },
    "CP-DATAFLOW-BATCH-VCPU": {
      "us": 0.056,
      "europe": 0.059,
      "asia": 0.059
    },
    "CP-DATAFLOW-STREAMING-VCPU": {
      "us": 0.069,
      "europe": 0.072,
      "asia": 0.072
    },
    "CP-DATAFLOW-BATCH-MEMORY": {
      "us": 0.003557,
      "europe": 0.004172,
      "asia": 0.004172
    },
    "CP-DATAFLOW-STREAMING-MEMORY": {
      "us": 0.003557,
      "europe": 0.004172,
      "asia": 0.004172
    },
    "CP-DATAFLOW-BATCH-STORAGE-PD": {
      "us": 0.000054,
      "europe": 0.000054,
      "asia": 0.000054
    },
    "CP-DATAFLOW-STREAMING-STORAGE-PD": {
      "us": 0.000054,
      "europe": 0.000054,
      "asia": 0.000054
    },
    "CP-DATAFLOW-BATCH-STORAGE-PD-SSD": {
      "us": 0.000298,
      "europe": 0.000298,
      "asia": 0.000298
    },
    "CP-DATAFLOW-STREAMING-STORAGE-PD-SSD": {
      "us": 0.000298,
      "europe": 0.000298,
      "asia": 0.000298
    },
    "CP-BIGTABLE-NODES": {
      "us": 0.65
    },
    "CP-BIGTABLE-SSD": {
      "us": 0.17
    },
    "CP-BIGTABLE-HDD": {
      "us": 0.026
    },
    "CP-PUB-SUB-OPERATIONS": {
      "tiers": {
        "250": 0.40,
        "750": 0.20,
        "1750": 0.10,
        "100000": 0.05
      }
    },
    "CP-COMPUTEENGINE-STATIC-IP-CHARGE": {
      "us": 0.01
    },
    "CP-COMPUTEENGINE-VPN": {
      "us": 0.05
    },
    "CP-DATAPROC": {
      "us": 0.01
    },
    "CP-CONTAINER-ENGINE-BASIC": {
      "us": 0,
      "us-central1": 0,
      "us-east1": 0,
      "us-east4": 0,
      "us-west1": 0,
      "europe": 0,
      "europe-west1": 0,
      "europe-west2": 0,
      "europe-west3": 0,
      "asia": 0,
      "asia-east": 0,
      "asia-northeast": 0,
      "asia-southeast": 0,
      "australia-southeast1": 0,
      "australia": 0,
      "southamerica-east1": 0,
      "asia-south1": 0
    },
    "CP-CONTAINER-ENGINE-STANDARD": {
      "us": 0,
      "us-central1": 0,
      "us-east1": 0,
      "us-east4": 0,
      "us-west1": 0,
      "europe": 0,
      "europe-west1": 0,
      "europe-west2": 0,
      "europe-west3": 0,
      "asia": 0,
      "asia-east": 0,
      "asia-northeast": 0,
      "asia-southeast": 0,
      "australia-southeast1": 0,
      "australia": 0,
      "southamerica-east1": 0,
      "asia-south1": 0
    },
    "CP-SUPPORT-BRONZE": {
      "us": 0.0
    },
    "CP-SUPPORT-SILVER": {
      "us": 150
    },
    "CP-SUPPORT-GOLD": {
      "us": 400,
      "schedule": {
        "10000": 0.09,
        "50000": 0.07,
        "200000": 0.05,
        "1000000000": 0.03
      }
    },
    "CP-COMPUTEENGINE-CUSTOM-VM-CORE": {
      "us": 0.033174,
      "us-central1": 0.033174,
      "us-east1": 0.033174,
      "us-east4": 0.037364,
      "us-west1": 0.033174,
      "europe": 0.036489,
      "europe-west1": 0.036489,
      "europe-west2": 0.040692,
      "europe-west3": 0.040692,
      "asia": 0.038410,
      "asia-east": 0.038410,
      "asia-northeast": 0.040618,
      "asia-southeast": 0.038996,
      "australia-southeast1": 0.04488,
      "australia": 0.04488,
      "southamerica-east1": 0.050190,
      "asia-south1": 0.037966
    },
    "CP-COMPUTEENGINE-CUSTOM-VM-RAM": {
      "us": 0.004446,
      "us-central1": 0.004446,
      "us-east1": 0.004446,
      "us-east4": 0.005008,
      "us-west1": 0.004446,
      "europe": 0.004892,
      "europe-west1": 0.004892,
      "europe-west2": 0.005453,
      "europe-west3": 0.005453,
      "asia": 0.005150,
      "asia-east": 0.005150,
      "asia-northeast": 0.005418,
      "asia-southeast": 0.005226,
      "australia-southeast1": 0.00601,
      "australia": 0.00601,
      "southamerica-east1": 0.006729,
      "asia-south1": 0.005088
    },
    "CP-COMPUTEENGINE-CUSTOM-VM-EXTENDED-RAM": {
      "us": 0.009550,
      "us-central1": 0.009550,
      "us-east1": 0.009550,
      "us-east4": 0.010756,
      "us-west1": 0.009550,
      "europe": 0.010506,
      "europe-west1": 0.010506,
      "europe-west2": 0.01171,
      "europe-west3": 0.01171,
      "asia": 0.011059,
      "asia-east": 0.011059,
      "asia-northeast": 0.011667,
      "asia-southeast": 0.011667,
      "australia-southeast1": 0.01291,
      "australia": 0.01291,
      "southamerica-east1": 0.014451,
      "asia-south1": 0.010929
    },
    "CP-COMPUTEENGINE-PREDEFINED-VM-CORE": {
      "us": 0.031611,
      "us-central1": 0.031611,
      "us-east1": 0.031611,
      "us-east4": 0.035605,
      "us-west1": 0.031611,
      "europe": 0.034773,
      "europe-west1": 0.034773,
      "europe-west2": 0.040730,
      "europe-west3": 0.040730,
      "asia": 0.036602,
      "asia-east": 0.036602,
      "asia-northeast": 0.040618,
      "asia-southeast": 0.038999,
      "australia-southeast1": 0.044856,
      "australia": 0.044856,
      "southamerica-east1": 0.0474165,
      "asia-south1": 0.0378409
    },
    "CP-COMPUTEENGINE-PREDEFINED-VM-RAM": {
      "us": 0.004237,
      "us-central1": 0.004237,
      "us-east1": 0.004237,
      "us-east4": 0.005605,
      "us-west1": 0.004237,
      "europe": 0.004661,
      "europe-west1": 0.004661,
      "europe-west2": 0.005411,
      "europe-west3": 0.005411,
      "asia": 0.004906,
      "asia-east": 0.004906,
      "asia-northeast": 0.005419,
      "asia-southeast": 0.005222,
      "australia-southeast1": 0.006048,
      "australia": 0.006048,
      "southamerica-east1": 0.0063555,
      "asia-south1": 0.005109
    },
    "CP-COMPUTEENGINE-CUSTOM-VM-CORE-PREEMPTIBLE": {
      "us": 0.00698,
      "us-central1": 0.00698,
      "us-east1": 0.00698,
      "us-east4": 0.007469,
      "us-west1": 0.00698,
      "europe": 0.00768,
      "europe-west1": 0.00768,
      "europe-west2": 0.00815,
      "europe-west3": 0.00815,
      "asia": 0.00768,
      "asia-east": 0.00768,
      "asia-northeast": 0.00883,
      "asia-southeast": 0.007801,
      "australia-southeast1": 0.00898,
      "australia": 0.00898,
      "southamerica-east1": 0.010038,
      "asia-south1": 0.007992
    },
    "CP-COMPUTEENGINE-CUSTOM-VM-RAM-PREEMPTIBLE": {
      "us": 0.00094,
      "us-central1": 0.00094,
      "us-east1": 0.00094,
      "us-east4": 0.001006,
      "us-west1": 0.00094,
      "europe": 0.00103,
      "europe-west1": 0.00103,
      "europe-west2": 0.00109,
      "europe-west3": 0.00109,
      "asia": 0.00103,
      "asia-east": 0.00103,
      "asia-northeast": 0.001178,
      "asia-southeast": 0.001045,
      "australia-southeast1": 0.00120,
      "australia": 0.00120,
      "southamerica-east1": 0.001346,
      "asia-south1": 0.001076
    },
    "CP-COMPUTEENGINE-CUSTOM-VM-EXTENDED-RAM-PREEMPTIBLE": {
      "us": 0.002014,
      "us-central1": 0.002014,
      "us-east1": 0.002014,
      "us-east4": 0.002155,
      "us-west1": 0.002014,
      "europe": 0.002212,
      "europe-west1": 0.002212,
      "europe-west2": 0.00235,
      "europe-west3": 0.00235,
      "asia": 0.002212,
      "asia-east": 0.002212,
      "asia-northeast": 0.002536,
      "asia-southeast": 0.002536,
      "australia-southeast1": 0.00258,
      "australia": 0.00258,
      "southamerica-east1": 0.002890,
      "asia-south1": 0.002306
    },
    "CP-COMPUTEENGINE-PREDEFINED-VM-CORE-PREEMPTIBLE": {
      "us": 0.0066551,
      "us-central1": 0.006655,
      "us-east1": 0.006655,
      "us-east4": 0.00712085,
      "us-west1": 0.006655,
      "europe": 0.007321,
      "europe-west1": 0.007321,
      "europe-west2": 0.00815,
      "europe-west3": 0.00815,
      "asia": 0.007320,
      "asia-east": 0.007320,
      "asia-northeast": 0.008830,
      "asia-southeast": 0.00780,
      "australia-southeast1": 0.0089,
      "australia": 0.0089,
      "southamerica-east1": 0.00998265,
      "asia-south1": 0.0080004
    },
    "CP-COMPUTEENGINE-PREDEFINED-VM-RAM-PREEMPTIBLE": {
      "us": 0.000892,
      "us-central1": 0.000892,
      "us-east1": 0.000892,
      "us-east4": 0.00095444,
      "us-west1": 0.000892,
      "europe": 0.000981,
      "europe-west1": 0.000981,
      "europe-west2": 0.00109,
      "europe-west3": 0.00109,
      "asia": 0.000981,
      "asia-east": 0.000981,
      "asia-northeast": 0.001178,
      "asia-southeast": 0.00105,
      "australia-southeast1": 0.00120,
      "australia": 0.00120,
      "southamerica-east1": 0.001338,
      "asia-south1": 0.00107454
    },
    "CP-DB-F1-MICRO": {
      "us": 0.0150,
      "us-central1": 0.0150,
      "us-east1": 0.0150,
      "us-east4": 0.0161,
      "us-west1": 0.0150,
      "europe": 0.0150,
      "europe-west1": 0.0150,
      "europe-west2": 0.0180,
      "europe-west3": 0.0180,
      "asia": 0.0150,
      "asia-east": 0.0150,
      "asia-northeast": 0.0195,
      "asia-southeast": 0,
      "australia-southeast1": 0.0203,
      "australia": 0.0203,
      "southamerica-east1": 0.0255,
      "asia-south1": 0.0180
    },
    "CP-DB-G1-SMALL": {
      "us": 0.0500,
      "us-central1": 0.0500,
      "us-east1": 0.0500,
      "us-east4": 0.0535,
      "us-west1": 0.0500,
      "europe": 0.0500,
      "europe-west1": 0.0500,
      "europe-west2": 0.0600,
      "europe-west3": 0.0600,
      "asia": 0.0500,
      "asia-east": 0.0500,
      "asia-northeast": 0.0650,
      "asia-southeast": 0,
      "australia-southeast1": 0.0675,
      "australia": 0.0675,
      "southamerica-east1": 0.0750,
      "asia-south1": 0.0600
    },
    "CP-DB-N1-STANDARD-1": {
      "us": 0.0965,
      "us-central1": 0.0965,
      "us-east1": 0.0965,
      "us-east4": 0.1033,
      "us-west1": 0.0965,
      "europe": 0.0965,
      "europe-west1": 0.0965,
      "europe-west2": 0.1158,
      "europe-west3": 0.1158,
      "asia": 0.0965,
      "asia-east": 0.0965,
      "asia-northeast": 0.1255,
      "asia-southeast": 0,
      "australia-southeast1": 0.1303,
      "australia": 0.1303,
      "southamerica-east1": 0.1448,
      "asia-south1": 0.1158
    },
    "CP-DB-N1-STANDARD-2": {
      "us": 0.1930,
      "us-central1": 0.1930,
      "us-east1": 0.1930,
      "us-east4": 0.2065,
      "us-west1": 0.1930,
      "europe": 0.1930,
      "europe-west1": 0.1930,
      "europe-west2": 0.2316,
      "europe-west3": 0.2316,
      "asia": 0.1930,
      "asia-east": 0.1930,
      "asia-northeast": 0.2509,
      "asia-southeast": 0,
      "australia-southeast1": 0.2606,
      "australia": 0.2606,
      "southamerica-east1": 0.2895,
      "asia-south1": 0.2316
    },
    "CP-DB-N1-STANDARD-4": {
      "us": 0.3860,
      "us-central1": 0.3860,
      "us-east1": 0.3860,
      "us-east4": 0.4130,
      "us-west1": 0.3860,
      "europe": 0.3860,
      "europe-west1": 0.3860,
      "europe-west2": 0.4632,
      "europe-west3": 0.4632,
      "asia": 0.3860,
      "asia-east": 0.3860,
      "asia-northeast": 0.5018,
      "asia-southeast": 0,
      "australia-southeast1": 0.5211,
      "australia": 0.5211,
      "southamerica-east1": 0.5790,
      "asia-south1": 0.4632
    },
    "CP-DB-N1-STANDARD-8": {
      "us": 0.7720,
      "us-central1": 0.7720,
      "us-east1": 0.7720,
      "us-east4": 0.8260,
      "us-west1": 0.7720,
      "europe": 0.7720,
      "europe-west1": 0.7720,
      "europe-west2": 0.9264,
      "europe-west3": 0.9264,
      "asia": 0.7720,
      "asia-east": 0.7720,
      "asia-northeast": 1.0036,
      "asia-southeast": 0,
      "australia-southeast1": 1.0422,
      "australia": 1.0422,
      "southamerica-east1": 1.1580,
      "asia-south1": 0.9264
    },
    "CP-DB-N1-STANDARD-16": {
      "us": 1.5445,
      "us-central1": 1.5445,
      "us-east1": 1.5445,
      "us-east4": 1.6526,
      "us-west1": 1.5445,
      "europe": 1.5445,
      "europe-west1": 1.5445,
      "europe-west2": 1.8534,
      "europe-west3": 1.8534,
      "asia": 1.5445,
      "asia-east": 1.5445,
      "asia-northeast": 2.0079,
      "asia-southeast": 0,
      "australia-southeast1": 2.0851,
      "australia": 2.0851,
      "southamerica-east1": 2.3168,
      "asia-south1": 1.8528
    },
    "CP-DB-N1-STANDARD-32": {
      "us": 3.0885,
      "us-central1": 3.0885,
      "us-east1": 3.0885,
      "us-east4": 3.3047,
      "us-west1": 3.0885,
      "europe": 3.0885,
      "europe-west1": 3.0885,
      "europe-west2": 3.7062,
      "europe-west3": 3.7062,
      "asia": 3.0885,
      "asia-east": 3.0885,
      "asia-northeast": 4.0151,
      "asia-southeast": 0,
      "australia-southeast1": 4.1695,
      "australia": 4.1695,
      "southamerica-east1": 4.6335,
      "asia-south1": 3.7056
    },
    "CP-DB-N1-STANDARD-64": {
      "us": 6.1770,
      "us-central1": 6.1770,
      "us-east1": 6.1770,
      "us-east4": 6.6094,
      "us-west1": 6.1770,
      "europe": 6.1770,
      "europe-west1": 6.1770,
      "europe-west2": 7.4124,
      "europe-west3": 7.4124,
      "asia": 6.1770,
      "asia-east": 6.1770,
      "asia-northeast": 8.0301,
      "asia-southeast": 0,
      "australia-southeast1": 8.3390,
      "australia": 8.3390,
      "southamerica-east1": 9.2655,
      "asia-south1": 7.4112
    },
    "CP-DB-N1-HIGHMEM-2": {
      "us": 0.2515,
      "us-central1": 0.2515,
      "us-east1": 0.2515,
      "us-east4": 0.2691,
      "us-west1": 0.2515,
      "europe": 0.2515,
      "europe-west1": 0.2515,
      "europe-west2": 0.3018,
      "europe-west3": 0.3018,
      "asia": 0.2515,
      "asia-east": 0.2515,
      "asia-northeast": 0.3270,
      "asia-southeast": 0,
      "australia-southeast1": 0.3395,
      "australia": 0.3395,
      "southamerica-east1": 0.3773,
      "asia-south1": 0.3018
    },
    "CP-DB-N1-HIGHMEM-4": {
      "us": 0.5030,
      "us-central1": 0.5030,
      "us-east1": 0.5030,
      "us-east4": 0.5382,
      "us-west1": 0.5030,
      "europe": 0.5030,
      "europe-west1": 0.5030,
      "europe-west2": 0.6036,
      "europe-west3": 0.6036,
      "asia": 0.5030,
      "asia-east": 0.5030,
      "asia-northeast": 0.6539,
      "asia-southeast": 0,
      "australia-southeast1": 0.6791,
      "australia": 0.6791,
      "southamerica-east1": 0.7545,
      "asia-south1": 0.6036
    },
    "CP-DB-N1-HIGHMEM-8": {
      "us": 1.0060,
      "us-central1": 1.0060,
      "us-east1": 1.0060,
      "us-east4": 1.0764,
      "us-west1": 1.0060,
      "europe": 1.0060,
      "europe-west1": 1.0060,
      "europe-west2": 1.2072,
      "europe-west3": 1.2072,
      "asia": 1.0060,
      "asia-east": 1.0060,
      "asia-northeast": 1.3078,
      "asia-southeast": 0,
      "australia-southeast1": 1.3581,
      "australia": 1.3581,
      "southamerica-east1": 1.509,
      "asia-south1": 1.2072
    },
    "CP-DB-N1-HIGHMEM-16": {
      "us": 2.0120,
      "us-central1": 2.0120,
      "us-east1": 2.0120,
      "us-east4": 2.1528,
      "us-west1": 2.0120,
      "europe": 2.0120,
      "europe-west1": 2.0120,
      "europe-west2": 2.4144,
      "europe-west3": 2.4144,
      "asia": 2.0120,
      "asia-east": 2.0120,
      "asia-northeast": 2.6156,
      "asia-southeast": 0,
      "australia-southeast1": 2.7162,
      "australia": 2.7162,
      "southamerica-east1": 3.0180,
      "asia-south1": 2.4144
    },
    "CP-DB-N1-HIGHMEM-32": {
      "us": 4.0240,
      "us-central1": 4.0240,
      "us-east1": 4.0240,
      "us-east4": 4.3057,
      "us-west1": 4.0240,
      "europe": 4.0240,
      "europe-west1": 4.0240,
      "europe-west2": 4.8288,
      "europe-west3": 4.8288,
      "asia": 4.0240,
      "asia-east": 4.0240,
      "asia-northeast": 5.2312,
      "asia-southeast": 0,
      "australia-southeast1": 5.4324,
      "australia": 5.4324,
      "southamerica-east1": 6.0360,
      "asia-south1": 4.8288
    },
    "CP-DB-N1-HIGHMEM-64": {
      "us": 8.0480,
      "us-central1": 8.0480,
      "us-east1": 8.0480,
      "us-east4": 8.6114,
      "us-west1": 8.0480,
      "europe": 8.0480,
      "europe-west1": 8.0480,
      "europe-west2": 9.6576,
      "europe-west3": 9.6576,
      "asia": 8.0480,
      "asia-east": 8.0480,
      "asia-northeast": 10.4624,
      "asia-southeast": 0,
      "australia-southeast1": 10.8648,
      "australia": 10.8648,
      "southamerica-east1": 12.0720,
      "asia-south1": 9.6576
    },
    "CP-CLOUDSQL-STORAGE-SSD": {
      "us": 0.17,
      "us-central1": 0.17,
      "us-east1": 0.17,
      "us-east4": 0.1819,
      "us-west1": 0.17,
      "europe": 0.17,
      "europe-west1": 0.17,
      "europe-west2": 0.204,
      "europe-west3": 0.204,
      "asia": 0.17,
      "asia-east": 0.17,
      "asia-northeast": 0.22,
      "asia-southeast": 0,
      "australia-southeast1": 0.230,
      "australia": 0.230,
      "southamerica-east1": 0.255,
      "asia-south1": 0.204
    },
    "CP-CLOUDSQL-BACKUP": {
      "us": 0.08,
      "us-central1": 0.08,
      "us-east1": 0.08,
      "us-east4": 0.0856,
      "us-west1": 0.08,
      "europe": 0.08,
      "europe-west1": 0.08,
      "europe-west2": 0.096,
      "europe-west3": 0.096,
      "asia": 0.08,
      "asia-east": 0.08,
      "asia-northeast": 0.10,
      "asia-southeast": 0,
      "australia-southeast1": 0.108,
      "australia": 0.108,
      "southamerica-east1": 0.12,
      "asia-south1": 0.096
    },
    "CP-VISION-LABEL-DETECTION": {
      "tiers": {
        "1000": 0,
        "5000000": 0.0015,
        "20000000": 0.001
      }
    },
    "CP-VISION-OCR": {
      "tiers": {
        "1000": 0,
        "5000000": 0.0015,
        "20000000": 0.00060
      }
    },
    "CP-VISION-EXPLICIT-CONTENT-DETECTION": {
      "tiers": {
        "1000": 0,
        "5000000": 0.0015,
        "20000000": 0.00060
      }
    },
    "CP-VISION-FACIAL-DETECTION": {
      "tiers": {
        "1000": 0,
        "5000000": 0.0015,
        "20000000": 0.00060
      }
    },
    "CP-VISION-LANDMARK-DETECTION": {
      "tiers": {
        "1000": 0,
        "5000000": 0.0015,
        "20000000": 0.00060
      }
    },
    "CP-VISION-LOGO-DETECTION": {
      "tiers": {
        "1000": 0,
        "5000000": 0.0015,
        "20000000": 0.00060
      }
    },
    "CP-VISION-IMAGE-PROPERTIES": {
      "tiers": {
        "1000": 0,
        "5000000": 0.0015,
        "20000000": 0.00060
      }
    },
    "CP-VISION-IMAGE-WEB-DETECTION": {
      "tiers": {
        "1000": 0,
        "5000000": 0.0035,
        "20000000": 0.0035
      }
    },
    "CP-VISION-IMAGE-DOCUMENT-TEXT-DETECTION": {
      "tiers": {
        "1000": 0,
        "5000000": 0.0035,
        "20000000": 0.0035
      }
    },
    "CP-STACKDRIVER-MONITORED-RESOURCES": {
      "us": 8.0
    },
    "CP-STACKDRIVER-LOGS-VOLUME": {
      "us": 0.50
    },
    "CP-STACKDRIVER-METRICS-DESCRIPTION": {
      "us": 1,
      "freequota" : {
        "quantity": 250
      }
    },
    "CP-STACKDRIVER-TIME-SERIES": {
      "us": 0.10
    },
    "CP-CLOUDCDN-CACHE-EGRESS-APAC": {
      "tiers": {
        "10240": 0.09,
        "153600": 0.06,
        "1024000": 0.05,
        "2048000": 0.04
      }
    },
    "CP-CLOUDCDN-CACHE-EGRESS-AU": {
      "tiers": {
        "10240": 0.11,
        "153600": 0.09,
        "1024000": 0.08,
        "2048000": 0.065
      }
    },
    "CP-CLOUDCDN-CACHE-EGRESS-CN": {
      "tiers": {
        "10240": 0.20,
        "153600": 0.17,
        "1024000": 0.16,
        "2048000": 0.145
      }
    },
    "CP-CLOUDCDN-CACHE-EGRESS-EU": {
      "tiers": {
        "10240": 0.08,
        "153600": 0.055,
        "1024000": 0.03,
        "2048000": 0.02
      }
    },
    "CP-CLOUDCDN-CACHE-EGRESS-NA": {
      "tiers": {
        "10240": 0.08,
        "153600": 0.055,
        "1024000": 0.03,
        "2048000": 0.02
      }
    },
    "CP-CLOUDCDN-CACHE-EGRESS-OTHER": {
      "tiers": {
        "10240": 0.09,
        "153600": 0.06,
        "1024000": 0.05,
        "2048000": 0.04
      }
    },
    "CP-CLOUDCDN-CACHE-FILL-INTRA-APAC": {
      "us": 0.06
    },
    "CP-CLOUDCDN-CACHE-FILL-INTRA-EU": {
      "us": 0.05
    },
    "CP-CLOUDCDN-CACHE-FILL-INTRA-NA": {
      "us": 0.04
    },
    "CP-CLOUDCDN-CACHE-FILL-INTER-AU": {
      "us": 0.15
    },
    "CP-CLOUDCDN-CACHE-FILL-INTER-OTHER": {
      "us": 0.08
    },
    "CP-CLOUDCDN-CACHE-LOOKUP-REQUESTS": {
      "us": 0.00000075
    },
    "CP-CLOUDCDN-CACHE-INVALIDATION": {
      "us": 0.005
    },
    "CP-SPEECH-API-RECOGNITION": {
      "tiers": {
        "60": 0,
        "1000000": 0.024,
        "10000000": 0.02,
        "100000000": 0.018
      }
    },
    "CP-NL-API-ENTITY-RECOGNITION": {
      "tiers": {
        "5000": 0,
        "1000000": 0.001,
        "5000000": 0.0005,
        "20000000": 0.00025
      }
    },
    "CP-NL-API-SENTIMENT-ANALYSIS": {
      "tiers": {
        "5000": 0,
        "1000000": 0.001,
        "5000000": 0.0005,
        "20000000": 0.00025
      }
    },
    "CP-NL-API-SYNTAX-ANALYSIS": {
      "tiers": {
        "5000": 0,
        "1000000": 0.0005,
        "5000000": 0.00025,
        "20000000": 0.000125
      }
    },
    "BIG_QUERY_FLAT_RATE_ANALYSIS": {
      "us": 20
    },
    "CP-ML-TRAINING": {
      "us": 0.49,
      "europe": 0.54,
      "asia": 0.54
    },
    "CP-ML-PREDICTION-ONLINE": {
      "us": 0.3,
      "europe": 0.348,
      "asia": 0.348
    },
    "CP-ML-PREDICTION-PROCESSING": {
      "us": 0.40,
      "europe": 0.44,
      "asia": 0.44
    },
    "CP-ML-PREDICTION-BATCH": {
      "us": 0.09262,
      "europe": 0.10744,
      "asia": 0.10744
    },
    "CP-BIGSTORE-STORAGE-COLDLINE": {
      "us": 0.007,
      "us-central1": 0.007,
      "us-east1": 0.007,
      "us-east4": 0.01,
      "us-west1": 0.007,
      "europe": 0.007,
      "europe-west1": 0.007,
      "europe-west2": 0.013,
      "europe-west3": 0.013,
      "asia-east": 0.007,
      "asia-northeast": 0.01,
      "australia-southeast1": 0.013,
      "australia": 0.013,
      "southamerica-east1": 0.014,
      "asia-south1": 0.013
    },
    "CP-BIGSTORE-DATA-RETRIEVAL-COLDLINE": {
      "us": 0.05
    },
    "CP-BIGSTORE-STORAGE-MULTI_REGIONAL": {
      "us": 0.026,
      "us-central1": 0.026,
      "us-east1": 0.026,
      "us-east4": 0.026,
      "us-west1": 0.026,
      "europe": 0.026,
      "europe-west1": 0.026,
      "europe-west2": 0.026,
      "europe-west3": 0.026,
      "asia-east": 0.026,
      "asia-northeast": 0.026,
      "australia-southeast1": 0.026,
      "australia": 0.026,
      "southamerica-east1": 0.026,
      "asia-south1": 0.026
    },
    "CP-BIGSTORE-STORAGE-REGIONAL": {
      "us": 0.02,
      "us-central1": 0.02,
      "us-east1": 0.02,
      "us-east4": 0.023,
      "us-west1": 0.02,
      "europe": 0.02,
      "europe-west1": 0.02,
      "europe-west2": 0.023,
      "europe-west3": 0.023,
      "asia-east": 0.02,
      "asia-northeast": 0.023,
      "australia-southeast1": 0.023,
      "australia": 0.023,
      "southamerica-east1": 0.035,
      "asia-south1": 0.023
    },
    "CP-BIGSTORE-STORAGE-NEARLINE": {
      "us": 0.01,
      "us-central1": 0.01,
      "us-east1": 0.01,
      "us-east4": 0.016,
      "us-west1": 0.01,
      "europe": 0.01,
      "europe-west1": 0.01,
      "europe-west2": 0.0160,
      "europe-west3": 0.0160,
      "asia-east": 0.01,
      "asia-northeast": 0.016,
      "australia-southeast1": 0.016,
      "australia": 0.016,
      "southamerica-east1": 0.0200,
      "asia-south1": 0.0160
    },
    "CP-GAE-FLEX-INSTANCE-CORE-HOURS": {
      "us": 0.0526
    },
    "CP-GAE-FLEX-INSTANCE-RAM": {
      "us": 0.0071
    },
    "CP-GAE-FLEX-STORAGE-PD-CAPACITY": {
      "us": 0.0400
    },
    "CP-KMS-KEY-VERSION": {
      "us": 0.06
    },
    "CP-KMS-CRYPTO-OPERATION": {
      "us": 0.000003
    },
    "CP-PUBSUB-MESSAGE-DELIVERY-BASIC": {
      "tiers": {
        "10": 0,
        "50000": 0.06,
        "150000": 0.05,
        "1000000": 0.04
      }
    },
    "GPU_NVIDIA_TESLA_K80": {
      "us": 0.45,
      "us-central1": 0.45,
      "us-east1": 0.45,
      "us-east4": 0,
      "us-west1": 0.45,
      "europe": 0.49,
      "europe-west1": 0.49,
      "europe-west2": 0,
      "europe-west3": 0,
      "asia-east": 0.49,
      "asia-northeast": 0,
      "asia-southeast": 0,
      "australia-southeast1": 0,
      "australia": 0,
      "southamerica-east1": 0,
      "asia-south1": 0
    },
    "GPU_NVIDIA_TESLA_P100": {
      "us": 1.46,
      "us-central1": 1.46,
      "us-east1": 1.46,
      "us-east4": 0,
      "us-west1": 1.46,
      "europe": 1.6,
      "europe-west1": 1.6,
      "europe-west2": 0,
      "europe-west3": 0,
      "asia-east": 1.6,
      "asia-northeast": 0,
      "asia-southeast": 0,
      "australia-southeast1": 0,
      "australia": 0,
      "southamerica-east1": 0,
      "asia-south1": 0
    },
    "GPU_NVIDIA_TESLA_K80-PREEMPTIBLE": {
      "us": 0.22,
      "us-central1": 0,
      "us-east1": 0.22,
      "us-east4": 0,
      "us-west1": 0.22,
      "europe": 0.22,
      "europe-west1": 0.22,
      "europe-west2": 0,
      "europe-west3": 0,
      "asia-east": 0.22,
      "asia-northeast": 0,
      "asia-southeast": 0,
      "australia-southeast1": 0,
      "australia": 0,
      "southamerica-east1": 0,
      "asia-south1": 0
    },
    "GPU_NVIDIA_TESLA_P100-PREEMPTIBLE": {
      "us": 0.73,
      "us-central1": 0,
      "us-east1": 0.73,
      "us-east4": 0,
      "us-west1": 0.73,
      "europe": 0.73,
      "europe-west1": 0.73,
      "europe-west2": 0,
      "europe-west3": 0,
      "asia-east": 0.73,
      "asia-northeast": 0,
      "asia-southeast": 0,
      "australia-southeast1": 0,
      "australia": 0,
      "southamerica-east1": 0,
      "asia-south1": 0
    },
    "CP-SPANNER-NODE": {
      "us": 0.90,
      "us-central1": 0.90,
      "europe-west1": 0.90,
      "asia-east1": 0.90,
      "asia-northeast1": 1.17,
      "nam3": 3.0,
      "nam-eur-asia1": 9.0
    },
    "CP-SPANNER-STORAGE-SSD": {
      "us": 0.30,
      "us-central1": 0.30,
      "europe-west1": 0.30,
      "asia-east1": 0.30,
      "asia-northeast1": 0.39,
      "nam3": 0.5,
      "nam-eur-asia1": 0.9
    },
    "CP-CLOUD-ENDPOINTS-REQUESTS": {
      "tiers": {
        "2": 0,
        "1000": 3.0,
        "10000": 1.5
      }
    },
    "CP-FUNCTIONS-GB-SECONDS": {
      "tiers": {
        "400000": 0,
        "400001": 0.0000025
      }
    },
    "CP-FUNCTIONS-GHZ-SECONDS": {
      "tiers": {
        "200000": 0,
        "200001": 0.0000100
      }
    },
    "CP-FUNCTIONS-EXECUTIONS": {
      "tiers": {
        "2000000": 0,
        "2000001": 0.0000004
      }
    },
    "CP-FUNCTIONS-BW-O": {
      "tiers": {
        "5": 0,
        "6": 0.12
      }
    },
    "CP-PROF-SVC-START-INF": {
      "us": 50000
    },
    "CP-PROF-SVC-PLAN-INF": {
      "us": 105000
    },
    "CP-PROF-SVC-START-DA": {
      "us": 50000
    },
    "CP-PROF-SVC-PLAN-DA": {
      "us": 105000
    },
    "CP-PROF-SVC-START-ML": {
      "us": 80000
    },
    "CP-PROF-SVC-PLAN-ML": {
      "us": 120000
    },
    "CP-PROF-SVC-START-APP": {
      "us": 50000
    },
    "CP-PROF-SVC-PLAN-APP": {
      "us": 105000
    },
    "GAPPS-PROF-SVC-ADA": {
      "us": 450000
    },
    "GAPPS-PROF-SVC-CSA": {
      "us": 60000
    },
    "GAPPS-PROF-SVC-CMAS": {
      "us": 25000
    },
    "GAPPS-PROF-SVC-PSC": {
      "us": 60000
    },
    "GAPPS-PROF-SVC-SA": {
      "us": 30000
    },
    "GAPPS-PROF-SVC-TSP": {
      "us": 25000
    },
    "CP-PROF-SVC-TAM": {
      "us": 12500
    },
    "CP-DLP-INSPECT-CONTENT": {
      "tiers": {
        "1": 0,
        "1024": 10,
        "10240": 5
      }
    },
    "CP-DLP-REDACT-CONTENT": {
      "tiers": {
        "1": 0,
        "1024": 10,
        "10240": 5
      }
    },
    "CP-DLP-INSPECT-STORAGE": {
      "tiers": {
        "1": 0,
        "1024": 6,
        "10240": 3
      }
    },
    "CP-NEW-SUPPORT-USERS-DEVELOPMENT": {
      "us": 100
    },
    "CP-NEW-SUPPORT-USERS-PRODUCTION": {
      "us": 250
    },
    "CP-NEW-SUPPORT-USERS-ON-CALL": {
      "us": 1500
    },
    "CP-DB-PG-F1-MICRO": {
      "us": 0.0150,
      "us-central1": 0.0150,
      "us-east1": 0.0150,
      "us-east4": 0.0161,
      "us-west1": 0.0150,
      "europe": 0.0150,
      "europe-west1": 0.0150,
      "europe-west2": 0.0180,
      "europe-west3": 0.0180,
      "asia": 0.0150,
      "asia-east": 0.0150,
      "asia-northeast": 0.0195,
      "asia-southeast": 0,
      "australia-southeast1": 0.0203,
      "australia": 0.0203,
      "southamerica-east1": 0.0225,
      "asia-south1": 0.00180
    },
    "CP-DB-PG-G1-SMALL": {
      "us": 0.0500,
      "us-central1": 0.0500,
      "us-east1": 0.0500,
      "us-east4": 0.0535,
      "us-west1": 0.0500,
      "europe": 0.0500,
      "europe-west1": 0.0500,
      "europe-west2": 0.0600,
      "europe-west3": 0.0600,
      "asia": 0.0500,
      "asia-east": 0.0500,
      "asia-northeast": 0.0650,
      "asia-southeast": 0,
      "australia-southeast1": 0.0675,
      "australia": 0.0675,
      "southamerica-east1": 0.0750,
      "asia-south1": 0.0600
    },
    "CP-DB-PG-CUSTOM-VM-CORE": {
      "us": 0.0590,
      "us-central1": 0.0590,
      "us-east1": 0.0590,
      "us-east4": 0.0631,
      "us-west1": 0.0590,
      "europe": 0.0590,
      "europe-west1": 0.0590,
      "europe-west2": 0.07080,
      "europe-west3": 0.07080,
      "asia": 0.0590,
      "asia-east": 0.0590,
      "asia-northeast": 0.0767,
      "asia-southeast": 0,
      "australia-southeast1": 0.0797,
      "australia": 0.0797,
      "southamerica-east1": 0.08850,
      "asia-south1": 0.07080
    },
    "CP-DB-PG-CUSTOM-VM-RAM": {
      "us": 0.0100,
      "us-central1": 0.0100,
      "us-east1": 0.0100,
      "us-east4": 0.0107,
      "us-west1": 0.0100,
      "europe": 0.0100,
      "europe-west1": 0.0100,
      "europe-west2": 0.01200,
      "europe-west3": 0.01200,
      "asia": 0.0100,
      "asia-east": 0.0100,
      "asia-northeast": 0.0130,
      "asia-southeast": 0,
      "australia-southeast1": 0.0135,
      "australia": 0.0135,
      "southamerica-east1": 0.01500,
      "asia-south1": 0.01200
    },
    "CP-CUD-1-YEAR-CPU": {
      "us": 0.019915,
      "us-central1": 0.019915,
      "us-east1": 0.019915,
      "us-east4": 0.021309,
      "us-west1": 0.019915,
      "europe": 0.021907,
      "europe-west1": 0.021907,
      "europe-west2": 0.025636,
      "europe-west3": 0.025636,
      "asia-east": 0.023059,
      "asia-northeast": 0.025589,
      "asia-southeast": 0.024567,
      "australia-southeast1": 0.028274,
      "australia": 0.028274,
      "southamerica-east1": 0.031620,
      "asia-south1": 0.023919
    },
    "CP-CUD-1-YEAR-RAM": {
      "us": 0.002669,
      "us-central1": 0.002669,
      "us-east1": 0.002669,
      "us-east4": 0.002856,
      "us-west1": 0.002669,
      "europe": 0.002936,
      "europe-west1": 0.002936,
      "europe-west2": 0.003435,
      "europe-west3": 0.003435,
      "asia-east": 0.003091,
      "asia-northeast": 0.003414,
      "asia-southeast": 0.003292,
      "australia-southeast1": 0.003786,
      "australia": 0.003786,
      "southamerica-east1": 0.004239,
      "asia-south1": 0.003205
    },
    "CP-CUD-3-YEAR-CPU": {
      "us": 0.014225,
      "us-central1": 0.014225,
      "us-east1": 0.014225,
      "us-east4": 0.015221,
      "us-west1": 0.014225,
      "europe": 0.015648,
      "europe-west1": 0.015648,
      "europe-west2": 0.0183114,
      "europe-west3": 0.0183114,
      "asia-east": 0.016471,
      "asia-northeast": 0.018278,
      "asia-southeast": 0.017548,
      "australia-southeast1": 0.0201960,
      "australia": 0.0201960,
      "southamerica-east1": 0.0225855,
      "asia-south1": 0.017085
    },
    "CP-CUD-3-YEAR-RAM": {
      "us": 0.001907,
      "us-central1": 0.001907,
      "us-east1": 0.001907,
      "us-east4": 0.00204,
      "us-west1": 0.001907,
      "europe": 0.002097,
      "europe-west1": 0.002097,
      "europe-west2": 0.0024539,
      "europe-west3": 0.0024539,
      "asia-east": 0.002208,
      "asia-northeast": 0.002438,
      "asia-southeast": 0.002352,
      "australia-southeast1": 0.0027045,
      "australia": 0.0027045,
      "southamerica-east1": 0.0030281,
      "asia-south1": 0.002290
    },
    "CP-DATAPREP-UNITS": {
      "us-central1": 0.096
    },
    "CP-IOT-CORE-DATA": {
      "tiers": {
        "250": 0,
        "256000": 0.0045,
        "5242880": 0.0020,
        "52428800": 0.00045
      }
    },
    "CP-VIDEO-INTELLIGENCE-LABEL-DETECTION": {
      "tiers": {
        "1000": 0,
        "100000": 0.1
      }
    },
    "CP-VIDEO-INTELLIGENCE-SHOT-DETECTION": {
      "tiers": {
        "1000": 0,
        "100000": 0.05
      }
    },
    "CP-VIDEO-INTELLIGENCE-EXPLICIT-CONTENT-DETECTION": {
      "tiers": {
        "1000": 0,
        "100000": 0.1
      }
    }
  }
}
`
