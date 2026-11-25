package dbviewer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Output protocol: one JSON document per line; mongoMoreMark means more rows
// existed than the limit, mongoErrPrefix carries the server's error text.
const (
	mongoMoreMark  = "LRMORE"
	mongoErrPrefix = "LRERR:"
)

// MongoResult holds the lines one Mongo operation produced.
type MongoResult struct {
	Documents  []json.RawMessage `json:"documents"`
	Truncated  bool              `json:"truncated"`
	DurationMs int64             `json:"duration_ms"`
}

// mongoCmd runs mongosh in the container. Credentials come from the
// container's own environment inside the script, never from an argument.
func mongoCmd() []string {
	return []string{"sh", "-c", "exec mongosh --quiet --norc --file /dev/stdin"}
}

func jsLit(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func jsParse(canonical string) string {
	return "EJSON.parse(" + jsLit(canonical) + ")"
}

// mongoScript assembles the script from validated parts and literals only.
func mongoScript(req MongoRequest, p mongoParts) string {
	var b strings.Builder
	b.WriteString(`(function(){try{`)
	b.WriteString(`if(process.env.MONGO_INITDB_ROOT_USERNAME){db.getSiblingDB("admin").auth(process.env.MONGO_INITDB_ROOT_USERNAME,process.env.MONGO_INITDB_ROOT_PASSWORD);}`)
	fmt.Fprintf(&b, `var MS=%d,CELL=%d,L=%d,SR=%d;`, p.lim.Timeout.Milliseconds(), p.lim.MaxCellBytes, p.limit, p.lim.SchemaRows)
	b.WriteString(`var E=function(o){return EJSON.stringify(o,null,0,{relaxed:true});};`)
	b.WriteString(`var P=function(o){var s=E(o);if(s.length>CELL){print(E({_id:o._id,"__truncated":true,bytes:s.length}));}else{print(s);}};`)
	b.WriteString(`var DRAIN=function(cur){var n=0;while(cur.hasNext()){var d=cur.next();n++;if(n>L){print("` + mongoMoreMark + `");break;}P(d);}};`)
	if req.Database != "" {
		fmt.Fprintf(&b, `var D=db.getSiblingDB(%s);`, jsLit(req.Database))
	}
	if req.Collection != "" {
		fmt.Fprintf(&b, `var C=D.getCollection(%s);`, jsLit(req.Collection))
	}
	switch req.Op {
	case MongoListDatabases:
		b.WriteString(`db.getMongo().getDBs().databases.forEach(function(x){print(JSON.stringify({name:x.name,sizeOnDisk:Number(x.sizeOnDisk),empty:!!x.empty}));});`)
	case MongoListCollections:
		b.WriteString(`D.getCollectionInfos({},true).slice(0,SR).forEach(function(x){print(JSON.stringify({name:x.name,type:x.type}));});`)
	case MongoCollections:
		b.WriteString(mongoOverviewScript(req.Database))
	case MongoFind:
		after := ""
		if p.after != "" {
			op := "$gt"
			if p.sort == `{"_id":-1}` {
				op = "$lt"
			}
			after = fmt.Sprintf(`F={$and:[F,{_id:{%s:%s}}]};`, op, jsParse(p.after))
		}
		sortSpec := p.sort
		if sortSpec == "{}" {
			sortSpec = `{"_id":1}`
		}
		fmt.Fprintf(&b, `var F=%s;%sDRAIN(C.find(F,%s).sort(%s).limit(L+1).maxTimeMS(MS));`,
			jsParse(p.filter), after, jsParse(p.proj), jsParse(sortSpec))
	case MongoCountDocuments:
		fmt.Fprintf(&b, `print(JSON.stringify({count:Number(C.countDocuments(%s,{maxTimeMS:MS}))}));`, jsParse(p.filter))
	case MongoEstimatedCount:
		b.WriteString(`print(JSON.stringify({count:Number(C.estimatedDocumentCount({maxTimeMS:MS}))}));`)
	case MongoGetIndexes:
		b.WriteString(`C.getIndexes().forEach(function(x){P(x);});`)
	case MongoStats:
		b.WriteString(`var S=D.runCommand({collStats:` + jsLit(req.Collection) + `});print(JSON.stringify({count:Number(S.count),size:Number(S.size),storageSize:Number(S.storageSize),avgObjSize:Number(S.avgObjSize||0),nindexes:Number(S.nindexes),totalIndexSize:Number(S.totalIndexSize)}));`)
	case MongoAggregate:
		fmt.Fprintf(&b, `DRAIN(C.aggregate(%s.concat([{$limit:L+1}]),{maxTimeMS:MS,allowDiskUse:false}));`, jsParse(p.pipeline))
	}
	b.WriteString(`}catch(e){print("` + mongoErrPrefix + `"+String(e.message||e));}})()` + "\n")
	return b.String()
}

// mongoOverviewScript lists databases, collections, counts, sizes and indexes
// with listing, count, stats and index reads only.
func mongoOverviewScript(database string) string {
	dbs := `DBS.map(function(x){return x.name;})`
	if database != "" {
		dbs = `[` + jsLit(database) + `]`
	}
	return `var left=SR,DBS=db.getMongo().getDBs().databases;` + dbs + `.forEach(function(dn){var d=db.getSiblingDB(dn);` +
		`print(JSON.stringify({kind:"db",name:dn,sizeOnDisk:Number((DBS.filter(function(x){return x.name===dn;})[0]||{}).sizeOnDisk||0)}));` +
		`d.getCollectionInfos({},true).forEach(function(ci){if(left<=0){return;}left--;` +
		`var o={kind:"collection",db:dn,name:ci.name,type:ci.type,count:0,size:0,storageSize:0,indexes:[]};` +
		`try{if(ci.type==="collection"){var c=d.getCollection(ci.name);o.count=Number(c.estimatedDocumentCount({maxTimeMS:MS}));` +
		`var s=d.runCommand({collStats:ci.name});o.size=Number(s.size||0);o.storageSize=Number(s.storageSize||0);` +
		`o.indexes=c.getIndexes().map(function(x){return {name:x.name,key:x.key,unique:!!x.unique};});}}catch(e){o.error=String(e.message||e);}` +
		`print(JSON.stringify(o));});});if(left<=0){print("` + mongoMoreMark + `");}`
}

// RunMongo executes a guard-approved query and collects its output lines.
func (t Target) RunMongo(ctx context.Context, q MongoQuery) (MongoResult, error) {
	ctx, cancel := context.WithTimeout(ctx, t.Limits.Timeout+timeoutMargin)
	defer cancel()

	start := time.Now()
	rc, err := t.Exec.ExecWithInput(ctx, t.ContainerID, mongoCmd(), strings.NewReader(q.script))
	if err != nil {
		return MongoResult{}, wrapExecError(ctx, err)
	}
	defer func() { _ = rc.Close() }()

	maxRows := t.Limits.MaxRows
	if q.Op == MongoCollections || q.Op == MongoListCollections {
		maxRows = t.Limits.SchemaRows + 1
	}
	capped := &capReader{r: rc, left: t.Limits.MaxBytes}
	sc := bufio.NewScanner(capped)
	// A cell is capped in the script at MaxCellBytes characters; allow for multibyte text.
	sc.Buffer(make([]byte, 0, 64<<10), 4*t.Limits.MaxCellBytes+4096)
	res := MongoResult{Documents: []json.RawMessage{}}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "":
		case line == mongoMoreMark:
			res.Truncated = true
		case strings.HasPrefix(line, mongoErrPrefix):
			return MongoResult{}, &QueryError{Message: strings.TrimPrefix(line, mongoErrPrefix)}
		case !json.Valid([]byte(line)):
			// mongosh banners or warnings, not data.
		case len(res.Documents) >= maxRows:
			res.Truncated = true
		default:
			res.Documents = append(res.Documents, json.RawMessage(line))
		}
		if res.Truncated && len(res.Documents) >= maxRows {
			break
		}
	}
	if capped.tripd {
		res.Truncated = true
	}
	if err := sc.Err(); err != nil && !capped.tripd {
		return MongoResult{}, wrapExecError(ctx, err)
	}
	if !res.Truncated {
		if _, err := io.Copy(io.Discard, rc); err != nil {
			return MongoResult{}, wrapExecError(ctx, err)
		}
	}
	res.DurationMs = time.Since(start).Milliseconds()
	return res, nil
}
