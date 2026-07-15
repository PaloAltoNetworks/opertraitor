package web

const FullHtmlTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>OperTraitor</title>
    <style>
        /* --- CSS RESET & VARIABLES --- */
        :root {
            --bg-color: #f0f2f5;
            --card-bg: #ffffff;
            --sidebar-bg: #1a1a1a;
            --text-primary: #333;
            --text-secondary: #666;
            --border-color: #ddd;
            --primary-blue: #007bff;
        }
        body { margin: 0; padding: 0; font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; background-color: var(--bg-color); color: var(--text-primary); }
        * { box-sizing: border-box; }
        a { text-decoration: none; color: inherit; }

        /* --- LAYOUT --- */
        .app { display: flex; min-height: 100vh; }
        
        /* Sidebar */
        .sidebar { 
            width: 250px; 
            background-color: var(--sidebar-bg); 
            color: white; 
            padding: 20px; 
            display: flex; 
            flex-direction: column; 
            position: fixed; 
            height: 100%; 
            overflow-y: auto; /* Allow scrolling if list is long */
        }
        .brand { font-size: 24px; font-weight: bold; margin-bottom: 30px; color: #fff; border-bottom: 1px solid #444; padding-bottom: 20px; }
        
        .menu-label { font-size: 11px; text-transform: uppercase; color: #666; font-weight: bold; margin-bottom: 10px; margin-top: 20px; letter-spacing: 1px; }
        
        .menu-item { padding: 10px 15px; color: #bbb; border-radius: 6px; margin-bottom: 2px; display: block; transition: 0.2s; font-size: 14px; }
        .menu-item:hover { background-color: #333; color: white; }
        .menu-item.active { background-color: var(--primary-blue); color: white; font-weight: bold; }

        /* Main Content */
        .content { margin-left: 250px; padding: 30px; width: 100%; }

        /* --- DASHBOARD STYLES --- */
        .stats-row { display: flex; gap: 20px; margin-bottom: 30px; }
        .stat-box { background: white; padding: 20px; border-radius: 8px; flex: 1; box-shadow: 0 2px 4px rgba(0,0,0,0.05); text-align: center; border: 1px solid var(--border-color); }
        .stat-num { font-size: 32px; font-weight: bold; color: #222; margin-top: 5px; }
        .stat-label { font-size: 12px; text-transform: uppercase; color: #888; letter-spacing: 1px; }

        .search-bar { width: 100%; padding: 15px; font-size: 16px; border: 1px solid #ccc; border-radius: 8px; margin-bottom: 30px; outline: none; }
        .search-bar:focus { border-color: var(--primary-blue); box-shadow: 0 0 5px rgba(0,123,255,0.3); }

        /* THE GRID (Cards) */
        .grid { 
            display: grid; 
            grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); 
            gap: 25px; 
        }

        /* THE CARD */
        .card {
            background-color: var(--card-bg);
            border-radius: 10px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.05);
            border: 1px solid #e0e0e0;
            transition: transform 0.2s, box-shadow 0.2s;
            overflow: hidden;
            position: relative;
            display: flex;
            flex-direction: column;
            height: 250px; /* Fixed height for uniformity */
        }
        .card:hover {
            transform: translateY(-5px);
            box-shadow: 0 12px 20px rgba(0,0,0,0.1);
            border-color: #ccc;
            cursor: pointer;
        }
        .card-header {
            padding: 20px;
            border-bottom: 1px solid #f0f0f0;
            background: #fafafa;
            display: flex;
            justify-content: space-between;
            align-items: start;
        }
        .card-body {
            padding: 20px;
            flex-grow: 1;
            overflow: hidden;
        }
        .card-footer {
            padding: 15px 20px;
            background: #fff;
            border-top: 1px solid #f0f0f0;
            font-size: 13px;
            color: #888;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        /* Card Typography */
        .op-name { font-size: 18px; font-weight: bold; margin: 0; color: #222; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
        .op-provider { font-size: 11px; text-transform: uppercase; color: #888; font-weight: bold; margin-bottom: 5px; }
        
        .op-desc { font-size: 14px; color: #555; line-height: 1.5; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
        
        /* Markdown / Description Styling */
        .description-content { font-size: 14px; color: #555; line-height: 1.6; }
        .description-content h1, .description-content h2, .description-content h3 { 
            margin-top: 1rem; margin-bottom: 0.5rem; font-size: 1.1em; color: #333; 
        }
        .description-content p { margin-bottom: 10px; }
        .description-content ul, .description-content ol { margin-left: 1.5rem; margin-bottom: 10px; }
        .description-content code { 
            background-color: #f4f4f4; padding: 2px 4px; border-radius: 3px; font-family: monospace; font-size: 0.9em;
        }
        .description-content pre { 
            background-color: #f4f4f4; padding: 10px; border-radius: 5px; overflow-x: auto; 
        }
        .description-content a { color: #007bff; text-decoration: underline; }

        /* Badges & Dots */
        .badge { display: inline-block; padding: 4px 8px; border-radius: 4px; font-size: 12px; font-weight: bold; background: #e0e0e0; color: #333; }
        .risk-dot { height: 35px; width: 35px; border-radius: 50%; display: flex; align-items: center; justify-content: center; color: white; font-weight: bold; font-size: 16px; box-shadow: 0 2px 4px rgba(0,0,0,0.2); }
        .meta-tag { background: #f0f2f5; padding: 5px 10px; border-radius: 4px; font-size: 13px; color: #555; display: inline-block; margin-right: 10px; }

        /* Warning Banner */
        .warning-box {
            background-color: #fff3cd;
            color: #856404;
            border: 1px solid #ffeeba;
            padding: 15px;
            border-radius: 8px;
            margin-bottom: 20px;
            display: flex;
            align-items: start;
        }
        .warning-icon { font-size: 24px; margin-right: 15px; line-height: 1; }
        .warning-content strong { display: block; margin-bottom: 5px; }

        /* --- DETAIL VIEW STYLES --- */
        .back-btn { display: inline-block; margin-bottom: 20px; font-weight: bold; color: #666; }
        .back-btn:hover { color: #000; }
        
        .detail-hero { background: white; padding: 40px; border-radius: 12px; border: 1px solid #ddd; margin-bottom: 30px; display: flex; justify-content: space-between; align-items: center; }
        .detail-score-box { text-align: center; }
        .big-score { font-size: 60px; font-weight: 800; line-height: 1; }
        
        .analysis-section { background: white; border-radius: 12px; border: 1px solid #ddd; overflow: hidden; margin-bottom: 30px; }
        .analysis-header { background: #f8f9fa; padding: 15px 25px; border-bottom: 1px solid #ddd; font-weight: bold; }
        .analysis-body { padding: 25px; }

        .perm-row { display: flex; border-bottom: 1px solid #eee; padding: 15px 0; }
        .perm-row:last-child { border-bottom: none; }
        .perm-left { width: 30%; padding-right: 20px; }
        .perm-right { width: 70%; padding-left: 20px; border-left: 1px solid #eee; color: #444; }
        
        .tag-res { color: #d63384; font-family: monospace; font-weight: bold; }
        .tag-verb { display: inline-block; background: #eee; padding: 2px 6px; border-radius: 3px; font-size: 12px; margin-right: 4px; text-transform: uppercase; border: 1px solid #ccc; }

        /* UTILS */
        .text-green { color: #10b981; }
        .text-orange { color: #f59e0b; }
        .text-red { color: #ef4444; }
    </style>
</head>
<body>

<div class="app">
    <div class="sidebar">
        <div class="brand">OperTraitor</div>
        
        <a href="/" class="menu-item {{if eq .CurrentCat ""}}active{{end}}">
            All Operators
        </a>

        {{if .AllCategories}}
        <div class="menu-label">Categories</div>
        {{range .AllCategories}}
            <a href="/?cat={{.}}" class="menu-item {{if eq $.CurrentCat .}}active{{end}}">
                {{.}}
            </a>
        {{end}}
        {{end}}
    </div>

    <div class="content">
        
        {{if not .IsDetail}}
            <h1>{{if .CurrentCat}}{{.CurrentCat}}{{else}}Dashboard{{end}}</h1>
            
            <form action="/" method="GET">
                {{if .CurrentCat}}<input type="hidden" name="cat" value="{{.CurrentCat}}">{{end}}
                <input type="text" name="q" class="search-bar" placeholder="Search operators..." value="{{.Query}}">
            </form>

            <div class="stats-row">
                <div class="stat-box">
                    <div class="stat-label">Total Operators</div>
                    <div class="stat-num">{{.Stats.Total}}</div>
                </div>
                <div class="stat-box">
                    <div class="stat-label">Analyzed</div>
                    <div class="stat-num">{{.Stats.Analyzed}}</div>
                </div>
                <div class="stat-box">
                    <div class="stat-label">High Risk</div>
                    <div class="stat-num text-red">{{.Stats.HighRisk}}</div>
                </div>
            </div>

            {{if eq (len .Items) 0}}
                <div style="text-align: center; color: #777; margin-top: 50px;">
                    <h2>No operators found.</h2>
                    <p>Try adjusting your search or category filter.</p>
                </div>
            {{else}}
            <div class="grid">
                {{range .Items}}
                <a href="/operator?id={{.ID | urlquery}}">
                    <div class="card">
                        <div class="card-header">
                            <div>
                                <div class="op-provider">{{.Metadata.Provider}}</div>
                                <div class="op-name" title="{{.OperatorName}}">{{.OperatorName}}</div>
                            </div>
                            {{if .Analysis}}
                                <div class="risk-dot" style="background-color: {{riskColor .Analysis.OverallRiskScore}}">
                                    {{.Analysis.OverallRiskScore}}
                                </div>
                            {{else}}
                                <div class="risk-dot" style="background-color: #ccc; color: #666;">?</div>
                            {{end}}
                        </div>
                        <div class="card-body">
                            <div class="op-desc description-content">
                                {{if .Analysis}}
                                    {{.Analysis.Summary | markdown}}
                                {{else if .Metadata.Description}}
                                    {{.Metadata.Description | markdown}}
                                {{else}}
                                    No description available.
                                {{end}}
                            </div>
                        </div>
                        <div class="card-footer">
                            <div>
                                <span class="badge">v{{.Version}}</span>
                                <span style="font-size: 11px; color: #999; margin-left: 5px;">{{formatDate .Metadata.CreatedAt}}</span>
                            </div>
                            <span style="color: #007bff; font-weight: bold;">View Details &rarr;</span>
                        </div>
                    </div>
                </a>
                {{end}}
            </div>
            {{end}}
        {{else}}
            {{with .Item}}
            <a href="/" class="back-btn">&larr; Back to Dashboard</a>

            {{if isOld .Metadata.CreatedAt}}
            <div class="warning-box">
                <div class="warning-icon">&#9888;</div>
                <div class="warning-content">
                    <strong>Potential Deprecation Warning</strong>
                    The latest operator version on OperatorHub is more than 3 years old ({{formatDate .Metadata.CreatedAt}}). 
                    It could be deprecated, or the vendor has moved to publishing Helm charts. 
                    It may be found in <a href="https://artifacthub.io" target="_blank" style="text-decoration: underline;">artifacthub.io</a> or in the vendor's page.
                </div>
            </div>
            {{end}}

            <div class="detail-hero">
                <div>
                    <h1 style="margin: 0 0 10px 0;">{{.OperatorName}}</h1>
                    <div style="color: #666; margin-bottom: 20px;">
                        <span class="badge" style="margin-right: 10px;">v{{.Version}}</span>
                        <span class="meta-tag">Created: {{formatDate .Metadata.CreatedAt}}</span>
                        {{.Metadata.Provider}} &bull; {{.Metadata.Repository}}
                    </div>
                    
                    {{if .Metadata.Categories}}
                    <div style="margin-bottom: 20px;">
                        {{range .Metadata.Categories}}
                            <span class="badge" style="background: #e7f1ff; color: #007bff; border: 1px solid #cce5ff; margin-right: 5px;">{{.}}</span>
                        {{end}}
                    </div>
                    {{end}}

                    <div class="description-content" style="max-width: 800px;">
                        {{.Metadata.Description | markdown}}
                    </div>
                </div>
                {{if .Analysis}}
                <div class="detail-score-box">
                    <div class="stat-label">Risk Score</div>
                    <div class="big-score" style="color: {{riskColor .Analysis.OverallRiskScore}}">{{.Analysis.OverallRiskScore}}</div>
                </div>
                {{end}}
            </div>

            {{if .Analysis}}
                <div class="analysis-section">
                    <div class="analysis-header">AI Risk Analysis</div>
                    <div class="analysis-body">
                        <p style="font-size: 18px; line-height: 1.6; color: #333;">{{.Analysis.Summary}}</p>
                    </div>
                </div>

                <div class="analysis-section">
                    <div class="analysis-header">Permission Breakdown</div>
                    <div style="padding: 0 25px;">
                        {{range .Analysis.RBACPermissionEvaluation}}
                        <div class="perm-row">
                            <div class="perm-left">
                                <div style="margin-bottom: 5px;"><strong>{{.Scope}}</strong> Access</div>
                                <div style="font-family: monospace; color: #007bff; margin-bottom: 5px;">{{.APIGroup}}</div>
                                <div class="tag-res" style="margin-bottom: 8px;">{{.Resources}}</div>
                                <div>{{range .Verbs}}<span class="tag-verb">{{.}}</span>{{end}}</div>
                            </div>
                            <div class="perm-right">
                                <div style="margin-bottom: 5px;">
                                    {{if eq .Permissiveness "Proper"}}
                                        <span style="color: #10b981; font-weight: bold;">&#10003; Proper</span>
                                    {{else}}
                                        <span style="color: #ef4444; font-weight: bold;">&#9888; Overly Permissive</span>
                                    {{end}}
                                     - Risk: <span style="font-weight: bold;">{{.RiskScore}}</span>
                                </div>
                                <p style="margin: 0; font-size: 14px;">{{.Reasoning}}</p>
                            </div>
                        </div>
                        {{end}}
                    </div>
                </div>
            {{end}}

            <details style="margin-top: 40px; border: 1px solid #ddd; padding: 15px; border-radius: 8px; background: white;">
                <summary style="font-weight: bold; cursor: pointer;">View Raw RBAC Rules</summary>
                <div style="margin-top: 15px; overflow-x: auto;">
                    <table style="width: 100%; border-collapse: collapse; font-family: monospace; font-size: 13px;">
                        <thead style="background: #eee;">
                            <tr><th style="padding:10px; text-align:left;">Scope</th><th style="padding:10px; text-align:left;">SA</th><th style="padding:10px; text-align:left;">API Groups</th><th style="padding:10px; text-align:left;">Resources</th><th style="padding:10px; text-align:left;">Verbs</th></tr>
                        </thead>
                        <tbody>
                            {{range .Rules}}
                            <tr style="border-bottom: 1px solid #eee;">
                                <td style="padding:10px;">{{.Scope}}</td>
                                <td style="padding:10px;">{{.ServiceAccount}}</td>
                                <td style="padding:10px; color: #007bff;">{{if .APIGroups}}{{.APIGroups}}{{else}}(core){{end}}</td>
                                <td style="padding:10px; color: #d63384;">{{.Resources}}</td>
                                <td style="padding:10px;">{{.Verbs}}</td>
                            </tr>
                            {{end}}
                        </tbody>
                    </table>
                </div>
            </details>
            {{end}}
        {{end}}
    </div>
</div>

</body>
</html>
`
