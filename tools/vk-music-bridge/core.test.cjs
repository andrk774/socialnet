const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
require('./core.js');
const c=global.SocialnetVK;
test('restrict destinations and audio sources',()=>{
 assert(c.destination('http://localhost:8080/music'));
 for(const u of ['http://localhost:9999','https://evil.test','http://127.0.0.1.evil.test:8080'])assert(!c.destination(u));
 assert(c.audioURL('https://a.vkuseraudio.net/music.mp3?x=1'));
 for(const u of ['https://vkuseraudio.net.evil.test/a.mp3','http://a.vkuseraudio.net/a.mp3','https://a.vkuseraudio.net/a.m3u8','https://u:p@a.vkuseraudio.net/a.mp3','http://127.0.0.1/a.mp3'])assert.equal(c.audioURL(u),null);
});
test('metadata and deduplication preserve playlist memberships',()=>{
 const a=c.track([12,-3,'https://a.vkuseraudio.net/a.mp3','Title','Artist']);
 assert.equal(a.key,'-3_12');
 const merged=c.merge(c.merge([], [a], 'One'),[{...a,url:null}],'Two');
 assert.equal(merged.length,1);assert.equal(merged[0].url,a.url);assert.deepEqual(merged[0].playlists,['One','Two']);
 assert.equal(c.track({id:'bad',owner_id:1,title:'x'}),null);
});
test('worker pairs only selected tab and hides source URLs',async()=>{
 let listener;const data={};
 const chrome={runtime:{id:'test',getURL:p=>'chrome-extension://test/'+p,onMessage:{addListener:f=>listener=f},onConnect:{addListener:()=>{}}},tabs:{get:async()=>({url:'http://localhost:8080/music'})},storage:{session:{get:async k=>({[k]:data[k]}),set:async o=>Object.assign(data,o),clear:async()=>{for(const k of Object.keys(data))delete data[k];}}}};
 const context={chrome,SocialnetVK:c,importScripts:()=>{},URL,AbortController,setTimeout,clearTimeout};
 vm.runInNewContext(fs.readFileSync(__dirname+'/background.js','utf8'),context);
 const send=(m,s)=>new Promise(resolve=>listener(m,s,resolve));
 const popup={id:'test',url:'chrome-extension://test/popup.html'};
 const page={id:'test',frameId:0,url:'http://localhost:8080/music',tab:{id:10}};
 assert((await send({op:'catalogue'},page)).error);
 await send({op:'collect',items:[c.track([1,2,'https://a.vkuseraudio.net/a.mp3','T','A'])],label:'P'},popup);
 await send({op:'allow',tabId:10,origin:'http://localhost:8080'},popup);
 const result=await send({op:'catalogue'},page);
 assert.equal(result.items[0].available,true);assert.equal(result.items[0].url,undefined);
 assert((await send({op:'catalogue'},{...page,tab:{id:11}})).error);
 assert((await send({op:'catalogue'},{...page,frameId:1})).error);
 await send({op:'clear'},popup);assert((await send({op:'catalogue'},page)).error);
});
