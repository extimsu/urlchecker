#!/usr/bin/env python3
"""Generate charts/urlchecker/dashboards/urlchecker.json.

Edit this file, not the JSON: python3 hack/gen-dashboard.py
CI fails when the committed JSON differs from what this writes.
"""
import json
import os

OUT = os.path.join(os.path.dirname(__file__), "..", "charts", "urlchecker", "dashboards", "urlchecker.json")
DS = {"type": "prometheus", "uid": "${datasource}"}
S = 'job=~"$job", instance=~"$instance"'
T = S + ', url=~"$target"'
H = S + ', host=~"$host"'

panels = []
state = {"id": 0, "y": 0, "x": 0, "rowh": 0}


def q(expr, legend="", instant=False, fmt=None, ref="A"):
    x = {"datasource": DS, "expr": expr, "refId": ref, "legendFormat": legend}
    if instant:
        x.update(instant=True, range=False)
    if fmt:
        x["format"] = fmt
    return x


def steps(*s):
    return {"mode": "absolute", "steps": [{"color": c, "value": v} for c, v in s]}


LOSS = steps(("green", None), ("yellow", 0.001), ("orange", 0.01), ("red", 0.05))
UPDOWN = [{"type": "value", "options": {"0": {"text": "DOWN", "color": "red"}, "1": {"text": "UP", "color": "green"}}}]


def place(p, w, h):
    """Lay panels out left to right, wrapping at 24 columns."""
    if state["x"] + w > 24:
        state["y"] += state["rowh"]
        state["x"], state["rowh"] = 0, 0
    state["id"] += 1
    p.update(id=state["id"], gridPos={"x": state["x"], "y": state["y"], "w": w, "h": h})
    p.setdefault("datasource", DS)
    state["x"] += w
    state["rowh"] = max(state["rowh"], h)
    panels.append(p)


def row(title, collapsed=False):
    if state["x"]:
        state["y"] += state["rowh"]
    state["x"], state["rowh"] = 0, 0
    state["id"] += 1
    panels.append({"type": "row", "id": state["id"], "title": title, "collapsed": collapsed,
                   "gridPos": {"x": 0, "y": state["y"], "w": 24, "h": 1}, "panels": []})
    state["y"] += 1


def stat(title, desc, expr, unit, th, w, decimals=None, mappings=None):
    d = {"unit": unit, "thresholds": th, "color": {"mode": "thresholds"}}
    if decimals is not None:
        d["decimals"] = decimals
    if mappings:
        d["mappings"] = mappings
    place({"type": "stat", "title": title, "description": desc, "targets": [q(expr, instant=True)],
           "fieldConfig": {"defaults": d, "overrides": []},
           "options": {"reduceOptions": {"calcs": ["lastNotNull"], "values": False}, "colorMode": "background",
                       "graphMode": "none", "textMode": "value", "justifyMode": "center"}}, w, 4)


def ts(title, desc, targets, unit, w=12, h=8, th=None, extra=None, stack=False):
    d = {"unit": unit, "min": 0, "custom": {"lineWidth": 1, "fillOpacity": 30 if stack else 0, "showPoints": "never",
                                            "spanNulls": False, "stacking": {"mode": "normal" if stack else "none"}}}
    if th:
        d["thresholds"] = th
        d["custom"]["thresholdsStyle"] = {"mode": "dashed"}
    if extra:
        d.update(extra)
    place({"type": "timeseries", "title": title, "description": desc, "targets": targets,
           "fieldConfig": {"defaults": d, "overrides": []},
           "options": {"legend": {"displayMode": "table", "placement": "right", "calcs": ["max", "lastNotNull"],
                                  "sortBy": "Max", "sortDesc": True},
                       "tooltip": {"mode": "multi", "sort": "desc"}}}, w, h)


def timeline(title, desc, targets, mappings, th, w=24, h=7):
    place({"type": "state-timeline", "title": title, "description": desc, "targets": targets,
           "fieldConfig": {"defaults": {"thresholds": th, "color": {"mode": "thresholds"}, "mappings": mappings}, "overrides": []},
           "options": {"showValue": "never", "mergeValues": True, "rowHeight": 0.8, "alignValue": "left",
                       "legend": {"showLegend": False}, "tooltip": {"mode": "single"}}}, w, h)


def loss(by, win):
    return (f'clamp_min(1 - sum by ({by}) (rate(urlchecker_icmp_response_duration_seconds_count{{{H}}}[{win}]))'
            f' / (sum by ({by}) (rate(urlchecker_icmp_requests_total{{{H}}}[{win}])) > 0), 0)')


def rtt(p, by, win="$__rate_interval"):
    return f'histogram_quantile({p}, sum by ({by}, le) (rate(urlchecker_icmp_response_duration_seconds_bucket{{{H}}}[{win}])))'


# ---- Overview
row("Overview")
stat("Targets up", "TCP, UDP, HTTP and DNS targets whose last check passed.",
     f'count(urlchecker_current_status{{{T}}} == 1) or vector(0)', "none", steps(("green", None)), 5)
stat("Targets down", "TCP, UDP, HTTP and DNS targets whose last check failed.",
     f'count(urlchecker_current_status{{{T}}} == 0) or vector(0)', "none", steps(("green", None), ("red", 1)), 5)
stat("Failed checks (5m)", "Share of checks that failed over the last 5 minutes.",
     f'sum(rate(urlchecker_failed_checks{{{T}}}[5m])) / sum(rate(urlchecker_total_checks{{{T}}}[5m]))',
     "percentunit", steps(("green", None), ("orange", 0.01), ("red", 0.1)), 5)
stat("Worst ICMP loss (5m)", "Highest packet loss of any instance → icmp:// target pair over the last 5 minutes.",
     f'max({loss("instance, host", "5m")})', "percentunit", LOSS, 5)
stat("Soonest certificate expiry", "Days until the first HTTPS certificate expires.",
     f'min(urlchecker_tls_cert_expiry_timestamp_seconds{{{T}}} - time()) / 86400', "d",
     steps(("red", None), ("orange", 7), ("green", 21)), 4, decimals=0)

# ---- Checks
row("Checks (TCP, UDP, HTTP, DNS)")
place({"type": "table", "title": "Target status",
       "description": "Last check result and the 95th percentile response time over 5 minutes, per instance and target.",
       "targets": [q(f'urlchecker_current_status{{{T}}}', instant=True, fmt="table", ref="A"),
                   q(f'histogram_quantile(0.95, sum by (instance, url, protocol, le) (rate(urlchecker_response_time_seconds_bucket{{{T}}}[5m])))',
                     instant=True, fmt="table", ref="B")],
       "transformations": [{"id": "merge", "options": {}},
                           {"id": "organize", "options": {"excludeByName": {"Time": True, "__name__": True, "job": True},
                                                          "indexByName": {"instance": 0, "url": 1, "protocol": 2, "Value #A": 3, "Value #B": 4},
                                                          "renameByName": {"url": "target", "Value #A": "status", "Value #B": "p95 response"}}}],
       "fieldConfig": {"defaults": {"custom": {"align": "auto", "filterable": True}}, "overrides": [
           {"matcher": {"id": "byName", "options": "status"}, "properties": [
               {"id": "mappings", "value": UPDOWN}, {"id": "custom.cellOptions", "value": {"type": "color-background"}}]},
           {"matcher": {"id": "byName", "options": "p95 response"}, "properties": [{"id": "unit", "value": "s"}]}]},
       "options": {"showHeader": True, "cellHeight": "sm", "sortBy": [{"displayName": "status", "desc": False}]}}, 12, 9)
ts("Response time, p95", "95th percentile check time per target: TCP connect, UDP, a full HTTP request, or a DNS lookup.",
   [q(f'histogram_quantile(0.95, sum by (instance, url, le) (rate(urlchecker_response_time_seconds_bucket{{{T}}}[$__rate_interval])))',
      "{{instance}} → {{url}}")], "s", h=9)
timeline("Status over time", "Check result per instance and target. Gaps mean no data.",
         [q(f'urlchecker_current_status{{{T}}}', "{{instance}} → {{url}}")], UPDOWN, steps(("red", None), ("green", 1)))
ts("Failed checks per second", "Failed checks per target.",
   [q(f'sum by (instance, url) (rate(urlchecker_failed_checks{{{T}}}[$__rate_interval]))', "{{instance}} → {{url}}")], "ops", w=24)

# ---- HTTP
row("HTTP(S)")
place({"type": "table", "title": "HTTP endpoints",
       "description": "Last status code, HTTP version, redirects followed, and days until the earliest certificate in the chain expires.",
       # max by (instance, url) drops __name__, so the merge can join the rows.
       "targets": [q(f'max by (instance, url) (urlchecker_http_status_code{{{T}}})', instant=True, fmt="table", ref="A"),
                   q(f'max by (instance, url) (urlchecker_http_version{{{T}}})', instant=True, fmt="table", ref="B"),
                   q(f'max by (instance, url) (urlchecker_http_redirects{{{T}}})', instant=True, fmt="table", ref="C"),
                   q(f'max by (instance, url) (urlchecker_tls_cert_expiry_timestamp_seconds{{{T}}} - time()) / 86400', instant=True, fmt="table", ref="D")],
       "transformations": [{"id": "merge", "options": {}},
                           {"id": "organize", "options": {"excludeByName": {"Time": True},
                                                          "indexByName": {"instance": 0, "url": 1},
                                                          "renameByName": {"url": "URL", "Value #A": "status", "Value #B": "HTTP",
                                                                           "Value #C": "redirects", "Value #D": "cert days left"}}}],
       "fieldConfig": {"defaults": {"custom": {"align": "auto", "filterable": True}}, "overrides": [
           {"matcher": {"id": "byName", "options": "status"}, "properties": [
               {"id": "thresholds", "value": steps(("green", None), ("orange", 400), ("red", 500))},
               {"id": "mappings", "value": [{"type": "value", "options": {"0": {"text": "no response", "color": "red"}}}]},
               {"id": "custom.cellOptions", "value": {"type": "color-text"}}]},
           {"matcher": {"id": "byName", "options": "HTTP"}, "properties": [
               {"id": "decimals", "value": 1},
               {"id": "mappings", "value": [{"type": "value", "options": {"0": {"text": "–"}}}]}]},
           {"matcher": {"id": "byName", "options": "cert days left"}, "properties": [
               {"id": "decimals", "value": 0},
               {"id": "mappings", "value": [{"type": "special", "options": {"match": "null", "result": {"text": "–", "color": "transparent"}}}]},
               {"id": "thresholds", "value": steps(("red", None), ("orange", 7), ("green", 21))},
               {"id": "custom.cellOptions", "value": {"type": "color-background"}}]}]},
       "options": {"showHeader": True, "cellHeight": "sm"}}, 12, 8)
ts("Request phases (stacked)", "Where the time of the last request went: DNS, connect, TLS, server processing (to the first byte) and transfer.",
   [q(f'urlchecker_http_phase_duration_seconds{{{T}}}', "{{url}} · {{phase}}")], "s", stack=True)

# ---- DNS
row("DNS")
ts("DNS answers", "Answers returned by the last lookup of each dns:// target. A drop to 0 means the name stopped resolving.",
   [q(f'urlchecker_dns_answers{{{T}}}', "{{instance}} → {{url}}")], "none", w=24, h=6)

# ---- ICMP
row("ICMP (icmp:// targets)")
ts("Packet loss", "Share of echo requests without a reply, per instance and target. smokeping_prober's formula: 1 − replies / requests.",
   [q(loss("instance, host", "$__rate_interval"), "{{instance}} → {{host}}")], "percentunit", th=LOSS, extra={"max": 1})
ts("Round trip, median", "Median echo round trip per instance and target.",
   [q(rtt("0.5", "instance, host"), "{{instance}} → {{host}}")], "s")
place({"type": "heatmap", "title": "Round-trip distribution",
       "description": "Echo round trips per bucket over time, smokeping-style: a wider or higher band means more jitter or latency. Use the ICMP target filter to look at one target.",
       "targets": [q(f'sum by (le) (increase(urlchecker_icmp_response_duration_seconds_bucket{{{H}}}[$__rate_interval]))', "{{le}}", fmt="heatmap")],
       "fieldConfig": {"defaults": {}, "overrides": []},
       "options": {"calculate": False, "yAxis": {"unit": "s", "axisPlacement": "left"},
                   "cellGap": 0, "color": {"mode": "scheme", "scheme": "Oranges", "steps": 64, "exponent": 0.5, "fill": "dark-orange"},
                   "filterValues": {"le": 1e-9}, "rowsFrame": {"layout": "auto"},
                   "tooltip": {"mode": "single", "yHistogram": True}, "legend": {"show": True}}}, 12, 8)
ts("Round trip, p99", "99th percentile echo round trip per instance and target. Spikes without loss often come before loss.",
   [q(rtt("0.99", "instance, host"), "{{instance}} → {{host}}")], "s")
place({"type": "table", "title": "Loss matrix (5m): instance ↓ × target →",
       "description": "One cell per pair. With one instance per node (DaemonSet), a bad node shows as a full row and column, and a bad path as a single cell.",
       "targets": [q(loss("instance, host", "5m"), instant=True, fmt="table")],
       "transformations": [{"id": "groupingToMatrix", "options": {"rowField": "instance", "columnField": "host", "valueField": "Value", "emptyValue": "null"}}],
       "fieldConfig": {"defaults": {"unit": "percentunit", "decimals": 2, "thresholds": LOSS, "color": {"mode": "thresholds"},
                                    "custom": {"align": "center", "cellOptions": {"type": "color-background", "mode": "basic"}}},
                       "overrides": [{"matcher": {"id": "byName", "options": "instance\\host"},
                                      "properties": [{"id": "displayName", "value": "instance"}, {"id": "custom.cellOptions", "value": {"type": "auto"}},
                                                     {"id": "custom.align", "value": "left"}]}]},
       "options": {"showHeader": True, "cellHeight": "sm"}}, 24, 8)

# ---- Exporter
row("Exporter")
stat("Last reload", "1 when the config and target files loaded; 0 when the last reload failed and the previous targets are still checked.",
     f'min(urlchecker_config_last_reload_successful{{{S}}})', "none",
     steps(("red", None), ("green", 1)), 6,
     mappings=[{"type": "value", "options": {"0": {"text": "FAILED"}, "1": {"text": "OK"}}}])
place({"type": "stat", "title": "Last successful reload", "description": "Time since the targets were last loaded.",
       "targets": [q(f'time() - max(urlchecker_config_last_reload_success_timestamp_seconds{{{S}}})', instant=True)],
       "fieldConfig": {"defaults": {"unit": "s", "thresholds": steps(("blue", None)), "color": {"mode": "thresholds"}}, "overrides": []},
       "options": {"reduceOptions": {"calcs": ["lastNotNull"], "values": False}, "colorMode": "value", "graphMode": "none", "textMode": "value"}}, 6, 4)
stat("Discovered targets", "Targets found by Kubernetes node discovery, summed over instances.",
     f'sum(urlchecker_discovery_targets{{{S}}}) or vector(0)', "none", steps(("blue", None)), 6)
stat("Discovery errors (1h)", "Failed node lists in the last hour. urlchecker keeps the last targets when a list fails.",
     f'sum(increase(urlchecker_discovery_errors_total{{{S}}}[1h])) or vector(0)', "none", steps(("green", None), ("red", 1)), 6, decimals=0)
place({"type": "table", "title": "Versions", "description": "urlchecker build per instance.",
       "targets": [q(f'urlchecker_build_info{{{S}}}', instant=True, fmt="table")],
       "transformations": [{"id": "organize", "options": {"excludeByName": {"Time": True, "__name__": True, "job": True, "Value": True}}}],
       "fieldConfig": {"defaults": {}, "overrides": []}, "options": {"showHeader": True, "cellHeight": "sm"}}, 24, 5)

ANY = '{__name__=~"urlchecker_total_checks|urlchecker_icmp_requests_total"'


def var(name, label, query):
    return {"name": name, "label": label, "type": "query", "datasource": DS, "definition": query,
            "query": {"qryType": 1, "query": query, "refId": name}, "multi": True, "includeAll": True,
            "allValue": ".*", "current": {"text": "All", "value": "$__all"}, "refresh": 2, "sort": 1}


dash = {
    "uid": "urlchecker", "title": "urlchecker", "tags": ["urlchecker", "blackbox", "icmp", "http", "dns"],
    "description": "TCP, UDP, ICMP, HTTP(S) and DNS checks from the urlchecker exporter: status, response time, "
                   "packet loss, round trip, HTTP phases and certificates. https://github.com/extimsu/urlchecker",
    "schemaVersion": 39, "version": 2, "editable": True, "graphTooltip": 1, "refresh": "1m",
    "time": {"from": "now-6h", "to": "now"}, "timezone": "",
    "links": [{"title": "urlchecker", "type": "link", "url": "https://github.com/extimsu/urlchecker", "targetBlank": True, "icon": "doc"}],
    "templating": {"list": [
        {"name": "datasource", "label": "Data source", "type": "datasource", "query": "prometheus", "current": {}, "refresh": 1, "hide": 0},
        var("job", "Job", f'label_values({ANY}}}, job)'),
        var("instance", "Instance", f'label_values({ANY}, job=~"$job"}}, instance)'),
        var("target", "Target", 'label_values(urlchecker_total_checks{job=~"$job", instance=~"$instance"}, url)'),
        var("host", "ICMP target", 'label_values(urlchecker_icmp_requests_total{job=~"$job", instance=~"$instance"}, host)'),
    ]},
    "annotations": {"list": [{"builtIn": 1, "datasource": {"type": "grafana", "uid": "-- Grafana --"}, "enable": True, "hide": True,
                              "iconColor": "rgba(0, 211, 255, 1)", "name": "Annotations & Alerts", "type": "dashboard"}]},
    "panels": panels,
}
with open(OUT, "w") as f:
    json.dump(dash, f, indent=2, ensure_ascii=False)
    f.write("\n")
