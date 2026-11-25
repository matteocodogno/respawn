// respawn: everything the deck does on top of demoit's navigation (timer, breaks, notes,
// search, export, the audience copy, polls, questions, exercises, themes...).
// Each slide is a full page, so everything here runs once per slide; state that must
// survive navigation (timer, breaks, sticky notes) lives in localStorage.
// window.RESPAWN (set by the server from respawn.yaml) carries the presentation's settings.
//
// This module is imported at the end of demoit.js, but ES imports are hoisted: it runs
// before demoit defines its custom elements. Thumbnail mode relies on that.

const root = document.documentElement;
const top = document.getElementById('top');
const params = new URLSearchParams(location.search);
const THUMB = params.has('thumb');
const PRINT = THUMB && params.has('print');
// Audience copy (served by the audience server): slides only, no terminal and no presenter tools.
const AUD = !!window.RESPAWN_AUDIENCE;
const CFG = window.RESPAWN || {};
// localStorage keys start with the presentation's namespace (respawn.yaml: namespace).
const NS = CFG.ns || 'respawn';
// Text shown to the room; respawn.yaml "strings" overrides any of these keys.
const T = (key, fallback) => (CFG.strings && CFG.strings[key]) || fallback;
const OFF = THUMB || AUD;
const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
const slideNo = typeof CurrentStep === 'number' ? CurrentStep : 0;          // eslint-disable-line no-undef
const slideCount = typeof StepCount === 'number' ? StepCount : 0;           // eslint-disable-line no-undef

const store = {
    get(key, fallback) {
        try { const v = localStorage.getItem(key); return v === null ? fallback : JSON.parse(v); } catch { return fallback; }
    },
    set(key, value) {
        try { if (value === null) localStorage.removeItem(key); else localStorage.setItem(key, JSON.stringify(value)); } catch { /* private mode */ }
    },
};

const el = (tag, cls, html) => {
    const n = document.createElement(tag);
    if (cls) n.className = cls;
    if (html !== undefined) n.innerHTML = html;
    return n;
};

const go = (n) => {
    const i = Math.max(0, Math.min(slideCount, n));
    window.location.href = i === 0 ? '/' : `/${i}`;
};

// ------------------------------------------------------------------ thumbnail mode
// Used by the progress-bar preview: no terminals, no animations, no speaker-notes sync.
if (THUMB) {
    document.querySelectorAll('web-term').forEach((t) => t.replaceWith(el('div', 'term-ghost', '<span>LIVE TERMINAL</span>')));
    window.BroadcastChannel = class { postMessage() {} close() {} set onmessage(_) {} };
    root.classList.add('thumb', 'ready');
    if (PRINT) root.classList.add('print');
}

// ------------------------------------------------------------------ print frame
// For the PDF export (and the print view) a slide must look exactly as on screen, frozen:
// once the fonts and the Mermaid diagrams are ready, every animation stops on one frame
// (loops in the middle of their cycle, entrances at their end) and __slideReady is set.
if (PRINT) {
    const fonts = document.fonts && document.fonts.ready ? document.fonts.ready : Promise.resolve();
    const diagrams = () => new Promise((ok) => {
        const t0 = Date.now();
        const check = () => {
            const pending = [...document.querySelectorAll('.mermaid')].some((m) => !m.querySelector('svg'));
            if (!pending || Date.now() - t0 > 6000) ok(); else setTimeout(check, 100);
        };
        check();
    });
    const freeze = () => {
        for (const a of document.getAnimations()) {
            try {
                const t = a.effect && a.effect.getComputedTiming ? a.effect.getComputedTiming() : null;
                if (t && t.iterations === Infinity) {
                    a.pause();
                    // the drifting sparks keep their own (random) place; other loops stop mid-cycle
                    if (a.animationName !== 'drift') a.currentTime = Math.max(0, t.delay || 0) + (Number(t.duration) || 0) / 2;
                } else {
                    a.finish();
                }
            } catch { /* an animation that cannot be moved stays as it is */ }
        }
    };
    Promise.all([fonts, diagrams()])
        .then(() => new Promise((ok) => requestAnimationFrame(() => requestAnimationFrame(ok))))
        .then(() => { freeze(); return new Promise((ok) => setTimeout(ok, 120)); })
        .then(() => { freeze(); window.__slideReady = true; });
}

// ------------------------------------------------------------------ entrance animations
const STAGGER = ['.grid', '.steps', '.flow', 'ul.list', 'ol.list', '.kv', '.cols', '.diagram', '.bars'];
for (const container of document.querySelectorAll(STAGGER.join(','))) {
    container.setAttribute('data-stagger', '');
    [...container.children].forEach((child, i) => child.style.setProperty('--i', i));
}
for (const tbody of document.querySelectorAll('.tbl tbody')) {
    [...tbody.children].forEach((tr, i) => tr.style.setProperty('--i', i));
}
for (const body of document.querySelectorAll('.slide .body')) {
    [...body.children].forEach((child, i) => {
        if (!child.style.getPropertyValue('--i')) child.style.setProperty('--i', i);
    });
}
document.querySelectorAll('.hero-title span').forEach((s, i) => s.style.setProperty('--i', i));

if (top && (!THUMB || PRINT) && !reduce) {
    const sparks = el('div', 'sparks');
    const colours = ['var(--yellow)', 'var(--magenta)', 'var(--cyan)', 'var(--orange)'];
    for (let i = 0; i < 26; i++) {
        const s = el('i');
        s.style.left = `${Math.random() * 100}%`;
        s.style.setProperty('--c', colours[i % colours.length]);
        s.style.setProperty('--d', `${7 + Math.random() * 9}s`);
        s.style.setProperty('--delay', `${-Math.random() * 16}s`);
        sparks.appendChild(s);
    }
    top.prepend(sparks);
}

if (top && !THUMB) top.appendChild(el('div', 'wipe'));

if (!THUMB) {
    const start = () => root.classList.add('anim', 'ready');
    if (document.fonts && document.fonts.ready) {
        Promise.race([document.fonts.ready, new Promise((r) => setTimeout(r, 800))]).then(start);
    } else {
        start();
    }
}

// ------------------------------------------------------------------ keyboard routing
// demoit navigates on any ArrowLeft/ArrowRight/Space on the document. Stop that when the
// presenter is typing, using a control, or an overlay is open.
const overlayOpen = () => document.querySelector('.ov.open');
const typing = (t, k) => t && (t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName)
    || (t.tagName === 'BUTTON' && (k === ' ' || k === 'Enter')));

const shortcuts = {};
window.addEventListener('keydown', (e) => {
    if (THUMB) { e.stopPropagation(); return; }
    if (typing(e.target, e.key)) { e.stopPropagation(); return; }
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    const handler = shortcuts[e.key];
    if (overlayOpen()) {
        e.stopPropagation();
        if (e.key === 'Escape' || handler) e.preventDefault();
        const ov = overlayOpen();
        if (ov && ov._keys && ov._keys[e.key]) { ov._keys[e.key](e); return; }
        if (e.key === 'Escape') { closeOverlays(); return; }
    }
    if (handler) {
        e.preventDefault();
        e.stopPropagation();
        handler(e);
    }
}, true);

const closeOverlays = () => {
    document.querySelectorAll('.ov.open').forEach((o) => o._close ? o._close() : o.classList.remove('open'));
};

// ------------------------------------------------------------------ live terminal
// Every tab of the terminal has the same controls in its own bar (demoit.js emits
// 'fw-action'): full screen shows that tab alone over the whole window, close ends
// that tab's shell; closing the last tab closes the dock and shows a TERMINAL button.
const dock = document.querySelector('.term-dock');
const slide = document.querySelector('.slide');

const tabsOf = () => {
    const term = dock && dock.querySelector('web-term');
    return term && term.shadowRoot ? [...term.shadowRoot.querySelectorAll('fake-window')] : [];
};

let touched = false;   // the presenter chose the terminal (click, t, full screen)

const setMax = (on, win) => {
    if (!dock) return;
    if (on) touched = true;
    const tabs = tabsOf();
    const target = win || tabs[0];
    tabs.forEach((t) => t.classList.toggle('hidden', on && t !== target));
    dock.classList.toggle('max', on);
    root.classList.toggle('term-max', on);
    const frame = target && target.querySelector('iframe');
    if (frame) setTimeout(() => frame.focus(), 60);
};

const closeTerminal = () => {
    if (!dock) return;
    setMax(false);
    const term = dock.querySelector('web-term');
    if (term) term.remove();                     // drops the iframes, the websockets and the shells
    slide.classList.add('term-closed');
};

const openTerminal = () => {
    if (!dock) return;
    slide.classList.remove('term-closed');
    if (!dock.querySelector('web-term')) {
        const t = document.createElement('web-term');
        t.setAttribute('path', dock.dataset.path || 'workspace');
        dock.appendChild(t);
    }
};

if (dock && !OFF) {
    // Never start a slide with the keyboard inside the terminal: keep focus on the slide
    // (so arrows navigate) until the presenter clicks into the terminal themselves.
    dock.addEventListener('pointerdown', () => { touched = true; }, true);
    document.body.setAttribute('tabindex', '-1');
    // Only while the slide loads: the terminal page grabs focus when it boots. A click
    // into the terminal happens inside its iframe (invisible to this page), so after the
    // grace period focus is left alone.
    const t0 = Date.now();
    const GRACE = 2000;
    const keepFocus = () => {
        if (touched || Date.now() - t0 > GRACE) return;
        const a = document.activeElement;
        if (a && a !== document.body && (a.tagName === 'WEB-TERM' || a.tagName === 'IFRAME' || a.closest?.('.term-dock'))) {
            a.blur();
            document.body.focus({ preventScroll: true });
        }
    };
    const guard = setInterval(keepFocus, 100);
    window.addEventListener('blur', () => setTimeout(keepFocus, 0));
    setTimeout(() => clearInterval(guard), GRACE + 100);
    window.addEventListener('load', () => { window.focus(); document.body.focus({ preventScroll: true }); });

    dock.dataset.path = (dock.querySelector('web-term') || {}).getAttribute?.('path') || 'workspace';
    const reopen = el('button', 'term-reopen', '<span class="ms">terminal</span> TERMINAL');
    reopen.type = 'button';
    reopen.addEventListener('click', openTerminal);
    slide.appendChild(reopen);

    document.addEventListener('fw-action', (ev) => {
        const { action, win } = ev.detail || {};
        if (action === 'max') setMax(!dock.classList.contains('max'), win);
        if (action === 'min') setMax(false);
        if (action === 'close') {
            if (tabsOf().length > 1 && win) {
                const wasMax = dock.classList.contains('max');
                win.remove();                     // close just that tab
                if (wasMax) setMax(false);
            } else {
                closeTerminal();
            }
        }
    });

    shortcuts.t = () => {
        if (slide.classList.contains('term-closed')) { openTerminal(); setTimeout(() => setMax(true), 50); return; }
        setMax(!dock.classList.contains('max'));
    };
    // Escape leaves full screen even when the overlay router is idle.
    window.addEventListener('keydown', (ev) => {
        if (ev.key === 'Escape' && dock.classList.contains('max')) { ev.stopPropagation(); setMax(false); }
    }, true);
}

// ------------------------------------------------------------------ reveal notes
document.addEventListener('click', (e) => {
    if (AUD) return;
    const card = e.target.closest('.card.has-reveal');
    if (card) { card.classList.toggle('open'); return; }
    const btn = e.target.closest('.reveal-btn');
    if (btn) { btn.parentElement.classList.toggle('open'); return; }
    const head = e.target.closest('.tbl.reveal-rows thead');
    if (head) {
        const tbl = head.closest('table');
        const all = [...tbl.querySelectorAll('tbody tr')];
        const show = all.some((r) => !r.classList.contains('shown'));
        all.forEach((r, i) => setTimeout(() => r.classList.toggle('shown', show), show ? i * 70 : 0));
        return;
    }
    const row = e.target.closest('.tbl.reveal-rows tbody tr');
    if (row) row.classList.toggle('shown');
});

// ------------------------------------------------------------------ next-token sampler
const softmax = (logits, temperature) => {
    const t = Math.max(temperature, 0.01);
    const max = Math.max(...logits);
    const exps = logits.map((l) => Math.exp((l - max) / t));
    const sum = exps.reduce((a, b) => a + b, 0);
    return exps.map((x) => x / sum);
};
const sampleIndex = (probs, r) => {
    let acc = 0;
    for (let i = 0; i < probs.length; i++) { acc += probs[i]; if (acc > r) return i; }
    return probs.length - 1;
};

for (const box of document.querySelectorAll('.nexttoken')) {
    const data = JSON.parse(box.dataset.nt);
    let temperature = data.temperature;
    let picks = [];
    let drawn = null;
    let tally = null;

    box.innerHTML = `
        <div class="nt-label label">INPUT · the sentence so far</div>
        <div class="nt-sentence"></div>
        <div class="nt-label label">LLM · next-token probabilities</div>
        <div class="nt-bars"></div>
        <div class="nt-controls">
            <label class="nt-temp">TEMPERATURE <b></b><input type="range" min="0.1" max="2" step="0.1"></label>
            <button type="button" class="btn" data-a="sample">SAMPLE TOKEN</button>
            <button type="button" class="btn alt" data-a="five">× 5 RUNS</button>
            <button type="button" class="btn ghost" data-a="reset">RESET</button>
        </div>`;
    const sentence = box.querySelector('.nt-sentence');
    const bars = box.querySelector('.nt-bars');
    const slider = box.querySelector('input');
    const tempOut = box.querySelector('.nt-temp b');
    slider.value = temperature;

    const render = () => {
        const words = picks.map((p, i) => `<b>${data.steps[i][p][0]}</b>`).join(' ');
        const complete = picks.length >= data.steps.length;
        sentence.innerHTML = `${data.prompt} ${words}${complete ? '' : '<i class="nt-cursor"></i>'}`;
        tempOut.textContent = Number(temperature).toFixed(1);
        box.querySelector('[data-a="sample"]').disabled = complete || drawn !== null;
        box.querySelector('[data-a="five"]').disabled = complete || drawn !== null;
        if (complete) {
            bars.innerHTML = '<p class="nt-done">Sentence complete. Raise the temperature and reset: stranger choices come out.</p>';
            return;
        }
        const cands = data.steps[picks.length];
        const probs = softmax(cands.map((c) => c[1]), temperature);
        bars.innerHTML = cands.map((c, i) => `
            <div class="nt-row${drawn === i ? ' pick' : ''}">
                <span class="nt-tok">${c[0]}</span>
                <span class="nt-track"><i style="width:${(probs[i] * 100).toFixed(1)}%"></i></span>
                <span class="nt-pct">${Math.round(probs[i] * 100)}%</span>
                <span class="nt-tally">${tally && tally[i] ? '●'.repeat(tally[i]) : ''}</span>
            </div>`).join('');
    };

    slider.addEventListener('input', () => { temperature = Number(slider.value); tally = null; render(); });
    slider.addEventListener('change', () => slider.blur());
    box.addEventListener('click', (e) => {
        const b = e.target.closest('button');
        if (!b || b.disabled) return;
        const cands = data.steps[picks.length];
        if (b.dataset.a === 'reset') { picks = []; drawn = null; tally = null; render(); return; }
        if (!cands) return;
        const probs = softmax(cands.map((c) => c[1]), temperature);
        if (b.dataset.a === 'five') {
            tally = cands.map(() => 0);
            for (let k = 0; k < 5; k++) tally[sampleIndex(probs, Math.random())] += 1;
            render();
            return;
        }
        tally = null;
        drawn = sampleIndex(probs, Math.random());
        render();
        setTimeout(() => { picks.push(drawn); drawn = null; render(); }, reduce ? 0 : 550);
    });
    render();
}

// ------------------------------------------------------------------ progress bar: jump + preview
const xp = document.querySelector('.foot .xp');
if (xp && top && !OFF) {
    const preview = el('div', 'thumb-pop', '<div class="thumb-frame"><iframe tabindex="-1" loading="eager"></iframe></div><span class="thumb-no"></span>');
    top.appendChild(preview);
    const frame = preview.querySelector('iframe');
    const label = preview.querySelector('.thumb-no');
    let pending = null;
    let shown = -1;

    const indexAt = (clientX) => {
        const r = xp.getBoundingClientRect();
        const f = Math.max(0, Math.min(1, (clientX - r.left) / r.width));
        return Math.round(f * slideCount);
    };

    xp.addEventListener('mousemove', (e) => {
        const n = indexAt(e.clientX);
        const tr = top.getBoundingClientRect();
        const w = preview.offsetWidth || 1;
        const x = Math.max(8, Math.min(tr.width - w - 8, e.clientX - tr.left - w / 2));
        preview.style.left = `${x}px`;
        preview.classList.add('on');
        frame.style.transform = `scale(${preview.querySelector('.thumb-frame').clientWidth / 1280})`;
        label.textContent = `LV ${n}/${slideCount}`;
        if (n === shown) return;
        clearTimeout(pending);
        pending = setTimeout(() => {
            shown = n;
            preview.classList.add('loading');
            frame.src = `/${n}?thumb=1`;
        }, 90);
    });
    frame.addEventListener('load', () => preview.classList.remove('loading'));
    xp.addEventListener('mouseleave', () => { preview.classList.remove('on'); clearTimeout(pending); });
    xp.addEventListener('click', (e) => go(indexAt(e.clientX)));
}

// ------------------------------------------------------------------ timer and breaks
// One clock shared by the slide timer and the break screens, persisted across slides.
const CLOCK = `${NS}.clock`;
const BREAKS = {
    coffee: { title: T('break.coffee.title', 'COFFEE BREAK'), sub: T('break.coffee.sub', 'Grab a coffee'), icon: 'coffee', minutes: Number(T('break.coffee.minutes', 15)) },
    lunch: { title: T('break.lunch.title', 'LUNCH BREAK'), sub: T('break.lunch.sub', 'Enjoy your meal'), icon: 'restaurant', minutes: Number(T('break.lunch.minutes', 60)) },
};

const fmt = (ms) => {
    const s = Math.max(0, Math.ceil(ms / 1000));
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    const sec = s % 60;
    return (h ? `${h}:${String(m).padStart(2, '0')}` : String(m).padStart(2, '0')) + ':' + String(sec).padStart(2, '0');
};
const hhmm = (t) => new Date(t).toLocaleTimeString(CFG.lang || undefined, { hour: '2-digit', minute: '2-digit' });

let clock = store.get(CLOCK, null);
const remaining = () => (!clock ? 0 : clock.running ? clock.end - Date.now() : clock.left);
// Breaks (c, p) also go to the audience copy (the audience server), which shows the room the
// same break screen and countdown. Timers stay on the presenter's screen.
const shareBreak = () => {
    if (AUD || THUMB) return;
    const brk = clock && BREAKS[clock.mode] ? {
        mode: clock.mode, running: !!clock.running, done: !!clock.done, total: clock.total,
        left: Math.max(0, remaining()), at: clock.running ? hhmm(clock.end) : '',
    } : null;
    fetch(`http://localhost:${(Number(location.port) || 80) + 2}/__break`, {
        method: 'POST', mode: 'no-cors', cache: 'no-store', headers: { 'Content-Type': 'text/plain' }, body: JSON.stringify(brk),
    }).catch(() => {});
};
const saveClock = () => { store.set(CLOCK, clock); shareBreak(); };

const newClock = (mode, minutes, label) => {
    const total = Math.round(minutes * 60 * 1000);
    clock = { mode, label: label || '', total, left: total, end: Date.now() + total, running: mode !== 'timer', view: 'full', done: false };
    saveClock();
    renderClock();
};

const toggleRun = () => {
    if (!clock) return;
    if (clock.running) { clock.left = clock.end - Date.now(); clock.running = false; }
    else { if (clock.left <= 0) clock.left = clock.total; clock.end = Date.now() + clock.left; clock.running = true; clock.done = false; }
    saveClock();
    renderClock();
};

const addMinutes = (m) => {
    if (!clock) return;
    const d = m * 60 * 1000;
    clock.total = Math.max(60 * 1000, clock.total + d);
    if (clock.running) clock.end = Math.max(Date.now(), clock.end + d);
    else clock.left = Math.max(0, clock.left + d);
    clock.done = false;
    saveClock();
    renderClock();
};

const resetClock = () => {
    if (!clock) return;
    clock.left = clock.total;
    clock.end = Date.now() + clock.total;
    clock.running = false;
    clock.done = false;
    saveClock();
    renderClock();
};

const dropClock = () => { clock = null; saveClock(); renderClock(); };
const setView = (v) => { if (clock) { clock.view = v; saveClock(); renderClock(); } };

// End-of-timer sound. If assets/media/timer-end.mp3 exists it is played (drop in any
// sound you are licensed to use); otherwise an original synth "level up" sting plays:
// noise whoosh, bass hit, rising square-wave arpeggio, shimmering final chord.
const synthSting = () => {
    const ctx = new (window.AudioContext || window.webkitAudioContext)();
    const t0 = ctx.currentTime + 0.02;
    const out = ctx.createDynamicsCompressor();
    const master = ctx.createGain();
    master.gain.value = 0.5;
    out.connect(master).connect(ctx.destination);

    // whoosh
    const len = Math.floor(ctx.sampleRate * 0.35);
    const buf = ctx.createBuffer(1, len, ctx.sampleRate);
    const data = buf.getChannelData(0);
    for (let i = 0; i < len; i++) data[i] = (Math.random() * 2 - 1) * (1 - i / len);
    const noise = ctx.createBufferSource();
    noise.buffer = buf;
    const bp = ctx.createBiquadFilter();
    bp.type = 'bandpass';
    bp.frequency.setValueAtTime(400, t0);
    bp.frequency.exponentialRampToValueAtTime(5000, t0 + 0.3);
    const ng = ctx.createGain();
    ng.gain.setValueAtTime(0.35, t0);
    ng.gain.exponentialRampToValueAtTime(0.001, t0 + 0.35);
    noise.connect(bp).connect(ng).connect(out);
    noise.start(t0);

    // bass hit
    const kick = ctx.createOscillator();
    const kg = ctx.createGain();
    kick.type = 'sine';
    kick.frequency.setValueAtTime(160, t0 + 0.3);
    kick.frequency.exponentialRampToValueAtTime(45, t0 + 0.6);
    kg.gain.setValueAtTime(0.9, t0 + 0.3);
    kg.gain.exponentialRampToValueAtTime(0.001, t0 + 0.75);
    kick.connect(kg).connect(out);
    kick.start(t0 + 0.3);
    kick.stop(t0 + 0.8);

    // rising arpeggio, two detuned squares per note
    const note = (freq, at, dur, vol) => {
        [-6, 6].forEach((detune) => {
            const o = ctx.createOscillator();
            const g = ctx.createGain();
            const lp = ctx.createBiquadFilter();
            o.type = 'square';
            o.frequency.value = freq;
            o.detune.value = detune;
            lp.type = 'lowpass';
            lp.frequency.value = 3200;
            g.gain.setValueAtTime(0.0001, at);
            g.gain.exponentialRampToValueAtTime(vol, at + 0.015);
            g.gain.exponentialRampToValueAtTime(0.0001, at + dur);
            o.connect(lp).connect(g).connect(out);
            o.start(at);
            o.stop(at + dur + 0.05);
        });
    };
    const arp = [392.0, 523.25, 659.25, 783.99, 1046.5];      // G4 C5 E5 G5 C6
    arp.forEach((f, i) => note(f, t0 + 0.32 + i * 0.075, 0.16, 0.12));
    // final chord with vibrato shimmer
    [523.25, 659.25, 783.99, 1046.5].forEach((f) => {
        const o = ctx.createOscillator();
        const g = ctx.createGain();
        const lfo = ctx.createOscillator();
        const lg = ctx.createGain();
        o.type = 'sawtooth';
        o.frequency.value = f;
        lfo.frequency.value = 7;
        lg.gain.value = f * 0.012;
        lfo.connect(lg).connect(o.frequency);
        const lp = ctx.createBiquadFilter();
        lp.type = 'lowpass';
        lp.frequency.value = 2400;
        const at = t0 + 0.32 + arp.length * 0.075;
        g.gain.setValueAtTime(0.0001, at);
        g.gain.exponentialRampToValueAtTime(0.07, at + 0.03);
        g.gain.exponentialRampToValueAtTime(0.0001, at + 1.1);
        o.connect(lp).connect(g).connect(out);
        o.start(at); lfo.start(at);
        o.stop(at + 1.2); lfo.stop(at + 1.2);
    });
};

let beeped = false;
const beep = () => {
    if (beeped || !soundOn()) return;
    beeped = true;
    const fallback = () => { try { synthSting(); } catch { /* no audio */ } };
    try {
        const audio = new Audio('/media/timer-end.mp3');
        audio.volume = 0.9;
        audio.addEventListener('error', fallback, { once: true });
        const p = audio.play();
        if (p && p.catch) p.catch(fallback);
    } catch { fallback(); }
};

// Full-screen panels (timer and breaks) and the corner widget.
const clockOv = el('div', 'ov clock-ov');
const widget = el('button', 'clock-mini');
widget.type = 'button';
if (top && !OFF) { top.appendChild(clockOv); top.appendChild(widget); }

clockOv._close = () => setView('mini');
clockOv._keys = {
    ' ': toggleRun,
    Enter: toggleRun,
    r: resetClock,
    ArrowUp: () => addMinutes(1),
    ArrowDown: () => addMinutes(-1),
    '+': () => addMinutes(5),
    '-': () => addMinutes(-5),
    Escape: () => setView('mini'),
    x: dropClock,
    Backspace: dropClock,
    m: () => setView('mini'),
    c: () => newClock('coffee', BREAKS.coffee.minutes),
    p: () => newClock('lunch', BREAKS.lunch.minutes),
    b: () => { beeped = false; beep(); },   // preview the end sound
};

clockOv.addEventListener('click', (e) => {
    const b = e.target.closest('button');
    if (!b) return;
    const a = b.dataset.a;
    if (a === 'run') toggleRun();
    if (a === 'reset') resetClock();
    if (a === 'plus') addMinutes(1);
    if (a === 'minus') addMinutes(-1);
    if (a === 'mini') setView('mini');
    if (a === 'stop') dropClock();
});
widget.addEventListener('click', () => setView('full'));

function renderClock() {
    if (OFF || !top) return;
    if (!clock) {
        clockOv.classList.remove('open');
        widget.classList.remove('on');
        return;
    }
    const brk = BREAKS[clock.mode];
    const left = remaining();
    // Minimised and nearly over: come back full screen for the last 30 seconds (once;
    // minimising again keeps it small, adding time re-arms it).
    if (left > 30000) clock.lastCall = false;
    else if (clock.running && left > 0 && clock.view === 'mini' && !clock.lastCall) {
        clock.lastCall = true;
        clock.view = 'full';
        saveClock();
    }
    const done = left <= 0 && (clock.running || clock.done);
    if (done && !clock.done) { clock.done = true; clock.running = false; clock.left = 0; saveClock(); beep(); }
    const frac = clock.total ? Math.max(0, Math.min(1, left / clock.total)) : 0;

    clockOv.classList.toggle('open', clock.view === 'full');
    clockOv.classList.toggle('is-break', !!brk);
    clockOv.classList.toggle('done', !!clock.done);
    clockOv.dataset.mode = clock.mode;
    widget.classList.toggle('on', clock.view === 'mini');
    widget.classList.toggle('done', !!clock.done);

    const back = brk && clock.running ? `${T('break.back', 'Back at')} <b>${hhmm(clock.end)}</b>` : brk ? T('break.paused', 'Paused') : '';
    const title = brk ? brk.title : (clock.label ? `TIMER · ${clock.label}` : 'TIMER');
    const key = `${clock.mode}|${clock.view}`;
    if (clockOv.dataset.key !== key) {
        clockOv.dataset.key = key;
        clockOv.innerHTML = `
            <div class="clock-card">
                ${brk ? `<div class="brk-icon"><span class="ms">${brk.icon}</span></div>` : ''}
                <div class="clock-title"></div>
                ${brk ? `<div class="brk-sub">${brk.sub}</div>` : ''}
                <div class="clock-digits"></div>
                <div class="clock-bar"><i></i></div>
                <div class="clock-back"></div>
                <div class="clock-btns">
                    <button type="button" class="btn" data-a="run"></button>
                    <button type="button" class="btn alt" data-a="minus">−1 MIN</button>
                    <button type="button" class="btn alt" data-a="plus">+1 MIN</button>
                    <button type="button" class="btn ghost" data-a="reset">RESET</button>
                    <button type="button" class="btn ghost" data-a="mini">MINIMISE</button>
                    <button type="button" class="btn ghost" data-a="stop">CLOSE</button>
                </div>
                <div class="clock-help">space start/pause · ↑↓ ±1 min · r reset · b test sound · esc minimise · x close</div>
            </div>`;
    }
    clockOv.querySelector('.clock-title').textContent = clock.done ? (brk ? "LET'S GO!" : "TIME'S UP!") : title;
    clockOv.querySelector('.clock-digits').textContent = fmt(left);
    clockOv.querySelector('.clock-bar i').style.width = `${(frac * 100).toFixed(2)}%`;
    clockOv.querySelector('.clock-back').innerHTML = back;
    clockOv.querySelector('[data-a="run"]').textContent = clock.running ? 'PAUSE' : (clock.done ? 'RESTART' : 'START');
    widget.innerHTML = `<span class="ms">${brk ? brk.icon : 'timer'}</span>${clock.done ? "TIME'S UP" : fmt(left)}${clock.running ? '' : ' ‖'}`;
}

let bindBadge = () => {};   // set below: makes a "7 MIN" badge start a timer
if (!OFF) {
    renderClock();
    if (clock && BREAKS[clock.mode]) shareBreak();
    setInterval(() => { if (clock && (clock.running || clock.view === 'full')) renderClock(); }, 250);

    // Clicking a "7 MIN" / "90 SEC" badge on a slide opens the timer already set.
    const parseBadge = (txt) => {
        const m = /(\d+(?:[.,]\d+)?)\s*(MIN|SEC)/i.exec(txt || '');
        if (!m) return null;
        const v = parseFloat(m[1].replace(',', '.'));
        return m[2].toUpperCase() === 'SEC' ? v / 60 : v;
    };
    bindBadge = (badge) => {
        const minutes = parseBadge(badge.textContent);
        if (minutes === null) return;
        badge.classList.add('clickable');
        badge.title = 'Start a timer';
        badge.addEventListener('click', () => newClock('timer', minutes, badge.textContent.replace('timer', '').trim()));
    };
    document.querySelectorAll('.timer').forEach(bindBadge);

    shortcuts.m = () => {
        if (clock && clock.mode === 'timer') { setView(clock.view === 'full' ? 'mini' : 'full'); return; }
        const badge = document.querySelector('.timer.clickable');
        if (badge) badge.click(); else newClock('timer', 5, '5 MIN');
    };
    shortcuts.c = () => newClock('coffee', BREAKS.coffee.minutes);
    shortcuts.p = () => newClock('lunch', BREAKS.lunch.minutes);
}

// ------------------------------------------------------------------ sticky notes
// "n" adds a note on the current slide; notes are draggable, editable and kept per slide.
// Keyed by deck and by slide id (not position), so the bootcamp and the full course,
// both served on localhost, keep their own notes and inserting a slide moves nothing.
const slideEl = document.querySelector('section.slide');
const NOTES = slideEl && slideEl.dataset.sid
    ? `${NS}.notes.${slideEl.dataset.track}.${slideEl.dataset.sid}`
    : `${NS}.notes.${slideNo}`;
const LEGACY_NOTES = `${NS}.notes.${slideNo}`;   // before the two decks: bootcamp, by position
const notesLayer = el('div', 'notes-layer');
if (top && !THUMB) top.appendChild(notesLayer);   // the audience keeps its own notes too

const saveNotes = () => {
    const data = [...notesLayer.querySelectorAll('.sticky')].map((n) => ({
        x: parseFloat(n.style.left), y: parseFloat(n.style.top), text: n.querySelector('.sticky-body').innerText, c: n.dataset.c,
    }));
    store.set(NOTES, data.length ? data : null);
};

const COLOURS = ['yellow', 'cyan', 'pink', 'green'];
const addSticky = (note, focus) => {
    const n = el('div', 'sticky',
        '<div class="sticky-bar"><span class="ms">drag_indicator</span><button type="button" class="sticky-c" title="Colour"><span class="ms">palette</span></button><button type="button" class="sticky-x" title="Delete"><span class="ms">close</span></button></div>' +
        '<div class="sticky-body" contenteditable="true" spellcheck="false"></div>');
    n.style.left = `${note.x}%`;
    n.style.top = `${note.y}%`;
    n.dataset.c = note.c || 'yellow';
    n.querySelector('.sticky-body').innerText = note.text || '';
    notesLayer.appendChild(n);

    const body = n.querySelector('.sticky-body');
    body.addEventListener('input', saveNotes);
    n.querySelector('.sticky-x').addEventListener('click', () => { n.remove(); saveNotes(); });
    n.querySelector('.sticky-c').addEventListener('click', () => {
        n.dataset.c = COLOURS[(COLOURS.indexOf(n.dataset.c) + 1) % COLOURS.length];
        saveNotes();
    });
    const bar = n.querySelector('.sticky-bar');
    bar.addEventListener('pointerdown', (e) => {
        if (e.target.closest('button')) return;
        const r = top.getBoundingClientRect();
        const sx = e.clientX; const sy = e.clientY;
        const ox = parseFloat(n.style.left); const oy = parseFloat(n.style.top);
        bar.setPointerCapture(e.pointerId);
        const move = (ev) => {
            n.style.left = `${Math.max(0, Math.min(88, ox + ((ev.clientX - sx) / r.width) * 100))}%`;
            n.style.top = `${Math.max(0, Math.min(85, oy + ((ev.clientY - sy) / r.height) * 100))}%`;
        };
        const up = () => { bar.removeEventListener('pointermove', move); bar.removeEventListener('pointerup', up); saveNotes(); };
        bar.addEventListener('pointermove', move);
        bar.addEventListener('pointerup', up);
    });
    if (focus) setTimeout(() => body.focus(), 30);
};

if (!THUMB) {
    let saved = store.get(NOTES, null);
    if (saved === null && slideEl && slideEl.dataset.track === 'bootcamp' && NOTES !== LEGACY_NOTES) {
        saved = store.get(LEGACY_NOTES, null);
        if (saved) { store.set(NOTES, saved); store.set(LEGACY_NOTES, null); }
    }
    (saved || []).forEach((n) => addSticky(n, false));
    shortcuts.n = () => {
        const k = notesLayer.children.length;
        addSticky({ x: 38 + (k % 4) * 3, y: 30 + (k % 4) * 4, text: '', c: COLOURS[k % COLOURS.length] }, true);
        saveNotes();
    };
    // Clicking outside a note leaves editing mode, so arrows navigate again.
    document.addEventListener('pointerdown', (e) => {
        if (!e.target.closest('.sticky') && document.activeElement && document.activeElement.isContentEditable) document.activeElement.blur();
    });
}

// ------------------------------------------------------------------ help
const help = el('div', 'ov help-ov', `
    <div class="help-card">
        <h2>CONTROLS</h2>
        <div class="help-groups">
            <section><h3>Move</h3><dl>
                <dt>→ ← Space</dt><dd>next / previous slide</dd>
                <dt>Home End</dt><dd>first / last slide</dd>
                <dt>/ f</dt><dd>search the slides</dd>
                <dt>s</dt><dd>speaker notes, every slide (a)</dd>
                <dt>Esc</dt><dd>close what is open</dd>
                <dt>?</dt><dd>this help</dd>
            </dl></section>
            <section><h3>Teach</h3><dl>
                <dt>t</dt><dd>terminal full screen</dd>
                <dt>m</dt><dd>timer (slide's minutes)</dd>
                <dt>c</dt><dd>coffee break</dd>
                <dt>p</dt><dd>lunch break</dd>
                <dt>n</dt><dd>sticky note on this slide</dd>
                <dt>e</dt><dd>export slides to PDF</dd>
            </dl></section>
            <section><h3>The room <small>respawn serve --audience</small></h3><dl>
                <dt>j</dt><dd>QR code to join</dd>
                <dt>l</dt><dd>hold the room here / release</dd>
                <dt>q</dt><dd>poll</dd>
                <dt>x</dt><dd>exercise catalog</dd>
                <dt>a</dt><dd>questions from the room</dd>
                <dt>Enter</dt><dd>quiz slide: start / close / leaderboard</dd>
                <dt>k</dt><dd>quiz leaderboard</dd>
            </dl></section>
            <section><h3>Fun</h3><dl>
                <dt>v</dt><dd>sound effects on / off</dd>
                <dt>g</dt><dd>shooting on / off</dd>
                <dt>h</dt><dd>the gif of respawn.yaml</dd>
                <dt>w</dt><dd>halloween edition on / off</dd>
            </dl></section>
        </div>
        <p>Click the progress bar to jump, hover it for a preview. Click a time badge to start a timer.</p>
    </div>`);
if (top && !OFF) {
    top.appendChild(help);
    help.addEventListener('click', () => help.classList.remove('open'));
    shortcuts['?'] = () => help.classList.toggle('open');
}

// ------------------------------------------------------------------ sound effects
// Original synth stings: a fanfare when a new module starts, a "pickup" whoosh when a
// new part (section) starts. "v" mutes/unmutes everything (remembered in the browser).
// Browsers only allow sound after the presenter has interacted with the deck; when a
// slide is reached with the keyboard or a click that is the case, otherwise the sting
// plays on the first click on the slide.
function soundOn() { return store.get(`${NS}.sound`, true) !== false; }

const withAudio = (play) => {
    const Ctx = window.AudioContext || window.webkitAudioContext;
    if (!Ctx) return;
    const ctx = new Ctx();
    let played = false;
    const go = () => { if (played) return; played = true; try { play(ctx); } catch { /* ignore */ } };
    if (ctx.state === 'running') { go(); return; }
    ctx.resume().then(() => { if (ctx.state === 'running') go(); }).catch(() => {});
    setTimeout(() => {
        if (ctx.state === 'running') return;
        const once = () => { ctx.resume().then(go); };
        document.addEventListener('pointerdown', once, { once: true, capture: true });
    }, 250);
};

const bus = (ctx, vol) => {
    const comp = ctx.createDynamicsCompressor();
    const g = ctx.createGain();
    g.gain.value = vol;
    comp.connect(g).connect(ctx.destination);
    return comp;
};

const noiseSweep = (ctx, out, at, dur, from, to, vol) => {
    const len = Math.floor(ctx.sampleRate * dur);
    const buf = ctx.createBuffer(1, len, ctx.sampleRate);
    const d = buf.getChannelData(0);
    for (let i = 0; i < len; i++) d[i] = (Math.random() * 2 - 1) * Math.sin(Math.PI * i / len);
    const src = ctx.createBufferSource();
    src.buffer = buf;
    const bp = ctx.createBiquadFilter();
    bp.type = 'bandpass';
    bp.Q.value = 1.2;
    bp.frequency.setValueAtTime(from, at);
    bp.frequency.exponentialRampToValueAtTime(to, at + dur);
    const g = ctx.createGain();
    g.gain.value = vol;
    src.connect(bp).connect(g).connect(out);
    src.start(at);
};

const thump = (ctx, out, at, vol) => {
    const o = ctx.createOscillator();
    const g = ctx.createGain();
    o.type = 'sine';
    o.frequency.setValueAtTime(150, at);
    o.frequency.exponentialRampToValueAtTime(42, at + 0.25);
    g.gain.setValueAtTime(vol, at);
    g.gain.exponentialRampToValueAtTime(0.001, at + 0.35);
    o.connect(g).connect(out);
    o.start(at);
    o.stop(at + 0.4);
};

const voice = (ctx, out, type, freqs, at, dur, vol, cutoff) => {
    freqs.forEach((f) => [-7, 7].forEach((det) => {
        const o = ctx.createOscillator();
        const g = ctx.createGain();
        const lp = ctx.createBiquadFilter();
        o.type = type;
        o.frequency.value = f;
        o.detune.value = det;
        lp.type = 'lowpass';
        lp.frequency.setValueAtTime(cutoff * 0.4, at);
        lp.frequency.exponentialRampToValueAtTime(cutoff, at + Math.min(0.25, dur / 2));
        g.gain.setValueAtTime(0.0001, at);
        g.gain.exponentialRampToValueAtTime(vol, at + 0.02);
        g.gain.setValueAtTime(vol, at + dur * 0.6);
        g.gain.exponentialRampToValueAtTime(0.0001, at + dur);
        o.connect(lp).connect(g).connect(out);
        o.start(at);
        o.stop(at + dur + 0.05);
    }));
};

// New module: "da-da-da-DAAA" brass fanfare over a riser, landing on a bright chord.
const moduleSting = (ctx) => {
    const out = bus(ctx, 0.55);
    const t = ctx.currentTime + 0.03;
    noiseSweep(ctx, out, t, 0.45, 300, 6000, 0.25);
    const G4 = 392.0, C5 = 523.25, E5 = 659.25, G5 = 783.99, C6 = 1046.5;
    [0, 0.13, 0.26].forEach((dt) => {
        voice(ctx, out, 'sawtooth', [G4, C5], t + 0.3 + dt, 0.11, 0.05, 2600);
        thump(ctx, out, t + 0.3 + dt, 0.5);
    });
    thump(ctx, out, t + 0.69, 0.9);
    voice(ctx, out, 'sawtooth', [C5, E5, G5, C6], t + 0.69, 1.3, 0.045, 3400);
    voice(ctx, out, 'square', [C6 * 2], t + 0.69, 0.9, 0.012, 6000);
};

// New part: quick whoosh and a two-note "pickup" chime.
const partSting = (ctx) => {
    const out = bus(ctx, 0.5);
    const t = ctx.currentTime + 0.03;
    noiseSweep(ctx, out, t, 0.3, 800, 7000, 0.22);
    voice(ctx, out, 'triangle', [1318.5], t + 0.22, 0.12, 0.09, 8000);   // E6
    voice(ctx, out, 'triangle', [1975.5], t + 0.32, 0.35, 0.08, 8000);   // B6
    voice(ctx, out, 'square', [987.77], t + 0.32, 0.2, 0.015, 4000);      // B5 body
};

if (!OFF && slide && soundOn()) {
    if (slide.classList.contains('module')) withAudio(moduleSting);
    else if (slide.classList.contains('part')) withAudio(partSting);
}

// Toast + toggle
const toast = (msg) => {
    if (!top) return;
    const t = el('div', 'toast', msg);
    top.appendChild(t);
    setTimeout(() => t.remove(), 1600);
};
if (!OFF) {
    shortcuts.v = () => {
        const on = !soundOn();
        store.set(`${NS}.sound`, on);
        toast(on ? '<span class="ms">volume_up</span> SOUND ON' : '<span class="ms">volume_off</span> SOUND OFF');
        if (on) withAudio(partSting);
    };
}

// ------------------------------------------------------------------ shooting gallery
// Clicking an empty part of a slide fires a shot: muzzle flash, small screen kick,
// a gunshot sound (if sound is on) and a bullet hole that slowly fades. "g" turns it off.
const NO_SHOOT = 'button, a, input, textarea, select, label, [contenteditable], .card.has-reveal, .reveal, '
    + '.tbl.reveal-rows, web-term, .term-dock, .sticky, .foot, .timer, .nexttoken, .ov, .clock-mini, '
    + '.thumb-pop, .term-reopen, .mermaid, .code.scroll, .corner-gif';
const gunOn = () => store.get(`${NS}.gun`, true) !== false;

let gunCtx = null;
const gunshot = () => {
    if (!soundOn()) return;
    try {
        const Ctx = window.AudioContext || window.webkitAudioContext;
        if (!gunCtx) gunCtx = new Ctx();
        const ctx = gunCtx;
        if (ctx.state !== 'running') ctx.resume();
        const t = ctx.currentTime + 0.005;
        const out = ctx.createDynamicsCompressor();
        const master = ctx.createGain();
        master.gain.value = 0.7;
        out.connect(master).connect(ctx.destination);
        // crack: short bright noise
        const len = Math.floor(ctx.sampleRate * 0.25);
        const buf = ctx.createBuffer(1, len, ctx.sampleRate);
        const d = buf.getChannelData(0);
        for (let i = 0; i < len; i++) d[i] = (Math.random() * 2 - 1) * Math.pow(1 - i / len, 3);
        const src = ctx.createBufferSource();
        src.buffer = buf;
        src.playbackRate.value = 0.85 + Math.random() * 0.3;
        const hp = ctx.createBiquadFilter();
        hp.type = 'highpass';
        hp.frequency.value = 900;
        const ng = ctx.createGain();
        ng.gain.setValueAtTime(1, t);
        ng.gain.exponentialRampToValueAtTime(0.001, t + 0.22);
        src.connect(hp).connect(ng).connect(out);
        src.start(t);
        // body: low thump
        const o = ctx.createOscillator();
        const og = ctx.createGain();
        o.type = 'triangle';
        o.frequency.setValueAtTime(140, t);
        o.frequency.exponentialRampToValueAtTime(40, t + 0.15);
        og.gain.setValueAtTime(0.9, t);
        og.gain.exponentialRampToValueAtTime(0.001, t + 0.2);
        o.connect(og).connect(out);
        o.start(t);
        o.stop(t + 0.25);
        // tail: filtered echo
        const tail = ctx.createBufferSource();
        tail.buffer = buf;
        const lp = ctx.createBiquadFilter();
        lp.type = 'lowpass';
        lp.frequency.value = 700;
        const tg = ctx.createGain();
        tg.gain.setValueAtTime(0.35, t + 0.06);
        tg.gain.exponentialRampToValueAtTime(0.001, t + 0.5);
        tail.connect(lp).connect(tg).connect(out);
        tail.start(t + 0.06);
    } catch { /* no audio */ }
};


// Cel-shaded hit, Borderlands-style: ink-outlined jagged hole with chunky cracks that
// stays a while, plus an impact star, an onomatopoeia and a floating damage number.
const rnd = (a, b) => a + Math.random() * (b - a);
const jagged = (cx, cy, rMin, rMax, n) => {
    const pts = [];
    for (let i = 0; i < n; i++) {
        const a = (i / n) * Math.PI * 2 + rnd(-0.12, 0.12);
        const r = rnd(rMin, rMax);
        pts.push(`${(cx + Math.cos(a) * r).toFixed(1)},${(cy + Math.sin(a) * r).toFixed(1)}`);
    }
    return pts.join(' ');
};
const star = (cx, cy, rIn, rOut, spikes) => {
    const pts = [];
    for (let i = 0; i < spikes * 2; i++) {
        const a = (i / (spikes * 2)) * Math.PI * 2;
        const r = i % 2 ? rIn * rnd(0.85, 1.1) : rOut * rnd(0.8, 1.1);
        pts.push(`${(cx + Math.cos(a) * r).toFixed(1)},${(cy + Math.sin(a) * r).toFixed(1)}`);
    }
    return pts.join(' ');
};

const holeSvg = () => {
    const cracks = [];
    const n = 4 + Math.floor(Math.random() * 2);
    for (let i = 0; i < n; i++) {
        const a = (i / n) * Math.PI * 2 + rnd(-0.35, 0.35);
        const r1 = 26, r2 = rnd(38, 44), r3 = rnd(50, 58);
        const k = rnd(-0.22, 0.22);
        const p = (r, da) => `${(60 + Math.cos(a + da) * r).toFixed(1)},${(60 + Math.sin(a + da) * r).toFixed(1)}`;
        cracks.push(`<polyline points="${p(r1, 0)} ${p(r2, k)} ${p(r3, k * 0.3)}" />`);
    }
    return `<svg viewBox="0 0 120 120" aria-hidden="true">
        <g class="cracks ink">${cracks.join('')}</g>
        <g class="cracks hi">${cracks.join('')}</g>
        <polygon class="rim" points="${star(60, 60, 21, 33, 9)}" />
        <polygon class="core" points="${jagged(60, 60, 12, 16, 10)}" />
        <path class="chip" d="M 50 49 Q 55 44 61 45" />
    </svg>`;
};

const WORDS = ['BANG!', 'POW!', 'BLAM!', 'KRAK!', 'BOOM!', 'ZAP!'];
const spawnHit = (layer, x, y) => {
    const place = (node) => { node.style.left = `${x}%`; node.style.top = `${y}%`; layer.appendChild(node); return node; };

    const hole = place(el('div', 'hole', holeSvg()));
    hole.style.setProperty('--rot', `${Math.floor(rnd(0, 360))}deg`);
    hole.style.setProperty('--s', rnd(0.85, 1.25).toFixed(2));

    const burst = place(el('div', 'burst', `<svg viewBox="0 0 120 120" aria-hidden="true">
        <polygon class="outer" points="${star(60, 60, 26, 56, 12)}" />
        <polygon class="inner" points="${star(60, 60, 14, 32, 9)}" /></svg>`));
    burst.style.setProperty('--rot', `${Math.floor(rnd(0, 360))}deg`);

    const word = place(el('div', 'bang', WORDS[Math.floor(Math.random() * WORDS.length)]));
    word.style.setProperty('--rot', `${rnd(-18, 18).toFixed(0)}deg`);
    word.style.setProperty('--dx', `${rnd(-4, 4).toFixed(1)}`);

    const crit = Math.random() < 0.2;
    const dmg = place(el('div', crit ? 'dmg crit' : 'dmg',
        `${crit ? '<small>CRITICAL!</small>' : ''}${Math.floor(crit ? rnd(400, 999) : rnd(24, 260))}`));
    dmg.style.setProperty('--dx', `${rnd(2, 6).toFixed(1)}`);

    setTimeout(() => burst.remove(), 320);
    setTimeout(() => word.remove(), 750);
    setTimeout(() => dmg.remove(), 1100);
    setTimeout(() => hole.classList.add('fade'), 9000);
    setTimeout(() => hole.remove(), 11000);
    while (layer.querySelectorAll('.hole').length > 40) layer.querySelector('.hole').remove();
};

if (top && !OFF) {
    const holes = el('div', 'holes');
    top.appendChild(holes);
    top.addEventListener('click', (e) => {
        if (!gunOn() || e.button !== 0) return;
        if (e.target.closest(NO_SHOOT)) return;
        if (window.getSelection && String(window.getSelection()).length) return;
        const r = top.getBoundingClientRect();
        const x = ((e.clientX - r.left) / r.width) * 100;
        const y = ((e.clientY - r.top) / r.height) * 100;

        spawnHit(holes, x, y);

        if (!reduce) {
            top.classList.remove('kick');
            void top.offsetWidth;            // restart the animation
            top.classList.add('kick');
        }
        gunshot();
    });
    shortcuts.g = () => {
        const on = !gunOn();
        store.set(`${NS}.gun`, on);
        toast(on ? '<span class="ms">my_location</span> SHOOTING ON' : '<span class="ms">block</span> SHOOTING OFF');
        if (!on) holes.innerHTML = '';
    };
}

// ------------------------------------------------------------------ corner gif
// "h" pops a gif in from a random corner of the screen (a comic panel that slides in,
// stays a few seconds and slides out; "h" again sends it away early). Set it in
// respawn.yaml (gif: media/whatever.gif, under the presentation's assets folder).
const GIF = CFG.gif ? `/${CFG.gif.replace(/^\/+/, '')}` : '';
if (top && !OFF) {
    let pop = null;
    let hideTimer = 0;
    const hide = () => {
        clearTimeout(hideTimer);
        if (!pop) return;
        const p = pop;
        pop = null;
        p.classList.remove('in');
        setTimeout(() => p.remove(), reduce ? 0 : 500);
    };
    shortcuts.h = () => {
        if (pop) { hide(); return; }
        const img = new Image();
        img.alt = '';
        img.onload = () => {
            const corner = ['tl', 'tr', 'bl', 'br'][Math.floor(Math.random() * 4)];
            pop = el('div', `corner-gif ${corner}`);
            pop.appendChild(img);
            pop.addEventListener('click', hide);
            top.appendChild(pop);
            void pop.offsetWidth;            // start from off screen
            pop.classList.add('in');
            hideTimer = setTimeout(hide, 5000);
        };
        img.onerror = () => toast('<span class="ms">image</span> SET gif: IN respawn.yaml');
        if (!GIF) { img.onerror(); return; }
        img.src = `${GIF}?t=${Date.now()}`;  // restart the animation every time
    };
}

// ------------------------------------------------------------------ audience copy
// the audience server serves a read-only copy of the deck to the room (slides only, up to the
// presenter's current slide). The presenter's page tells it which slide is current; the
// audience page follows along when it is on the live slide and offers a LIVE chip otherwise.
// Two ports above demoit's (8888 -> 8890, 8889 -> 8891), as respawn serve --audience uses.
const AUDIENCE_PORT = (Number(location.port) || 80) + 2;

if (!AUD && !THUMB) {
    // The audience server may not be running: a failed beacon is fine.
    fetch(`http://localhost:${AUDIENCE_PORT}/__live/set?n=${slideNo}`, { mode: 'no-cors', cache: 'no-store' }).catch(() => {});
}

if (AUD && top) {
    root.classList.add('audience');
    let live = window.RESPAWN_AUDIENCE.live;
    const following = slideNo >= live;
    const chip = el('a', 'live-chip', '');
    chip.href = '#';
    // "l" on the presenter's side: the room is held while the trainer looks at other slides.
    // The presenter's break screen (c, p), read-only: countdown and when we start again.
    const brkOv = el('div', 'ov clock-ov is-break aud-break', '');
    top.appendChild(brkOv);
    let brkEnd = 0, brkState = null;
    // Built once per break (and once more at "LET'S GO!"), then only the numbers change:
    // rebuilding it every tick would replay its entrance animation over and over.
    const paintBreak = () => {
        const b = brkState;
        if (!b) return;
        const info = BREAKS[b.mode] || BREAKS.coffee;
        const left = b.running ? Math.max(0, brkEnd - Date.now()) : b.left;
        const done = b.done || (b.running && left <= 0);
        const sig = `${b.mode}|${done}`;
        if (brkOv.dataset.sig !== sig) {
            brkOv.dataset.sig = sig;
            brkOv.dataset.mode = b.mode;
            brkOv.classList.toggle('done', done);
            brkOv.innerHTML = `<div class="clock-card">
                <div class="brk-icon"><span class="ms">${info.icon}</span></div>
                <div class="clock-title">${done ? "LET'S GO!" : info.title}</div>
                <div class="brk-sub">${info.sub}</div>
                <div class="clock-digits"></div>
                <div class="clock-bar"><i></i></div>
                <div class="clock-back"></div>
            </div>`;
        }
        const set = (sel, prop, v) => { const n = brkOv.querySelector(sel); if (n && n[prop] !== v) n[prop] = v; };
        set('.clock-digits', 'textContent', fmt(left));
        set('.clock-back', 'innerHTML', b.running && b.at && !done ? `${T('break.back', 'Back at')} <b>${b.at}</b>` : done ? '' : T('break.paused', 'Paused'));
        const bar = brkOv.querySelector('.clock-bar i');
        if (bar) bar.style.width = `${b.total ? Math.max(0, Math.min(100, (100 * left) / b.total)).toFixed(2) : 0}%`;
    };
    const showBreak = (b) => {
        brkState = b;
        if (!b) { brkOv.classList.remove('open'); brkOv.dataset.sig = ''; return; }
        // The server sends what is left, not a time of day; re-sync only when it drifts,
        // so the seconds do not jump back and forth at every poll.
        const end = Date.now() + (b.left || 0);
        if (!b.running || Math.abs(end - brkEnd) > 1500) brkEnd = end;
        brkOv.classList.add('open');
        paintBreak();
    };
    setInterval(paintBreak, 500);
    const hold = el('div', 'hold-chip aud-hold', '<span class="ms">pause_circle</span>ON HOLD · THE TRAINER IS LOOKING AT OTHER SLIDES');
    hold.hidden = true;
    top.appendChild(hold);
    chip.addEventListener('click', (ev) => { ev.preventDefault(); location.href = `/${live}`; });
    top.appendChild(chip);
    const show = () => {
        chip.innerHTML = `<i></i>LIVE · ${live}`;
        chip.hidden = slideNo >= live;
    };
    show();
    const poll = () => fetch('/__live', { cache: 'no-store' })
        .then((r) => r.json())
        .then((d) => {
            if (typeof d.n !== 'number') return;
            if (window.RESPAWN_SHOW_EXERCISE) window.RESPAWN_SHOW_EXERCISE(d.ex || null);
            hold.hidden = !d.held;
            showBreak(d.brk || null);
            if (window.RESPAWN_THEME) window.RESPAWN_THEME(d.theme || '');
            if (window.RESPAWN_QUESTION) window.RESPAWN_QUESTION(d.question || null);
            if (following && d.n !== slideNo) { location.href = `/${d.n}`; return; }
            live = d.n;
            if (slideNo > live) { location.href = `/${live}`; return; }
            show();
        })
        .catch(() => {})
        .finally(() => setTimeout(poll, 1000));
    setTimeout(poll, 300);
    // demoit moves one slide per keypress; past the live slide the server sends you back anyway.
    const FORWARD = ['ArrowRight', ' ', 'PageDown', 'End'];
    window.addEventListener('keydown', (ev) => {
        if (typing(ev.target, ev['ke' + 'y'])) return;
        if (FORWARD.includes(ev['ke' + 'y']) && slideNo >= live) { ev.preventDefault(); ev.stopPropagation(); }
    }, true);
}

// ------------------------------------------------------------------ search
// "/" (or "f") searches every slide of this deck: titles, text, code and, for the
// presenter, the speaker notes. The index is built with the deck (src/build.py) into
// assets/media/search-<deck>.json; the audience copy only gets slides up to the live one.
const TRACK = (document.querySelector('section.slide') || { dataset: {} }).dataset.track || CFG.deck || 'deck';
const fold = (s) => String(s || '').normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
const escHtml = (s) => String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));

let searchIndex = null;
const loadIndex = () => searchIndex || (searchIndex = fetch(`/media/search-${TRACK}.json`, { cache: 'no-store' })
    .then((r) => (r.ok ? r.json() : []))
    .then((rows) => rows.map((r) => ({ ...r, fk: fold(r.k), ft: fold(r.t), fx: fold(r.x), fo: fold(r.o) })))
    .catch(() => []));

const snippet = (text, words) => {
    const f = fold(text);
    let at = -1;
    for (const w of words) { const i = f.indexOf(w); if (i >= 0 && (at < 0 || i < at)) at = i; }
    if (at < 0) return escHtml(text.slice(0, 140));
    const start = Math.max(0, at - 60);
    const part = text.slice(start, start + 170);
    let out = '';
    const fp = fold(part);
    let i = 0;
    while (i < part.length) {
        const w = words.find((x) => x && fp.startsWith(x, i));
        if (w) { out += `<mark>${escHtml(part.slice(i, i + w.length))}</mark>`; i += w.length; } else { out += escHtml(part[i]); i += 1; }
    }
    return (start > 0 ? '…' : '') + out + (start + 170 < text.length ? '…' : '');
};

const searchOv = el('div', 'ov search-ov', `
    <div class="search-card" role="dialog" aria-label="Search the slides">
        <div class="search-bar"><span class="ms">search</span>
            <input id="rsp-search" type="search" placeholder="Search the slides…" autocomplete="off" spellcheck="false">
            <span class="search-count"></span></div>
        <ol class="search-results"></ol>
        <p class="search-hint">↑ ↓ to move · Enter to open · Esc to close</p>
    </div>`);
const sInput = searchOv.querySelector('input');
const sList = searchOv.querySelector('.search-results');
const sCount = searchOv.querySelector('.search-count');
let sHits = [];
let sSel = 0;

const renderHits = () => {
    sList.innerHTML = sHits.map((h, i) => `
        <li class="${i === sSel ? 'sel' : ''}" data-n="${h.n}">
            <span class="sr-n">${h.n}</span>
            <span class="sr-main"><span class="sr-k">${escHtml(h.k || '')}</span>
            <span class="sr-t">${escHtml(h.t || '(untitled)')}</span>
            <span class="sr-x">${h.snip}</span></span>
        </li>`).join('');
    const cur = sList.querySelector('.sel');
    if (cur) cur.scrollIntoView({ block: 'nearest' });
};
const runSearch = () => {
    const words = fold(sInput.value).split(/\s+/).filter(Boolean);
    if (!words.length) { sHits = []; sCount.textContent = ''; renderHits(); return; }
    loadIndex().then((rows) => {
        const hits = [];
        for (const r of rows) {
            const hay = `${r.fk} ${r.ft} ${r.fx} ${AUD ? '' : r.fo}`;
            if (!words.every((w) => hay.includes(w))) continue;
            let score = 0;
            for (const w of words) score += (r.ft.includes(w) ? 6 : 0) + (r.fk.includes(w) ? 2 : 0) + (r.fx.includes(w) ? 1 : 0);
            const inText = words.some((w) => r.fx.includes(w));
            hits.push({ ...r, score, snip: snippet(inText || AUD ? r.x : r.o, words) });
        }
        hits.sort((a, b) => b.score - a.score || a.n - b.n);
        sHits = hits.slice(0, 40);
        sSel = 0;
        sCount.textContent = hits.length ? `${hits.length}` : 'no match';
        renderHits();
    });
};
const openHit = (n) => { if (n !== undefined) location.href = `/${n}`; };
const openSearch = () => {
    closeOverlays();
    searchOv.classList.add('open');
    loadIndex();
    setTimeout(() => { sInput.focus(); sInput.select(); }, 20);
};
searchOv._close = () => { searchOv.classList.remove('open'); sInput.blur(); };

if (top && !THUMB) {
    top.appendChild(searchOv);
    sInput.addEventListener('input', runSearch);
    // On window, capture phase: the keyboard router above stops key events headed for
    // inputs before they reach them (so demoit does not navigate), but not this listener.
    window.addEventListener('keydown', (ev) => {
        if (ev.target !== sInput) return;
        const k = ev['ke' + 'y'];
        if (k === 'ArrowDown') { ev.preventDefault(); sSel = Math.min(sSel + 1, sHits.length - 1); renderHits(); }
        else if (k === 'ArrowUp') { ev.preventDefault(); sSel = Math.max(sSel - 1, 0); renderHits(); }
        else if (k === 'Enter') { ev.preventDefault(); if (sHits[sSel]) openHit(sHits[sSel].n); }
        else if (k === 'Escape') { ev.preventDefault(); searchOv._close(); }
    }, true);
    sList.addEventListener('click', (ev) => { const li = ev.target.closest('li'); if (li) openHit(li.dataset.n); });
    searchOv.addEventListener('click', (ev) => { if (ev.target === searchOv) searchOv._close(); });
    shortcuts['/'] = openSearch;
    shortcuts.f = openSearch;
}

// ------------------------------------------------------------------ export to PDF
// "e" asks for a range of slides and opens a print view of exactly those slides:
// choose "Save as PDF" in the print dialog. Presenter only.
const exportOv = el('div', 'ov export-ov', `
    <form class="export-card" aria-label="Export slides to PDF">
        <h2>EXPORT TO PDF</h2>
        <div class="export-range">
            <label>From <input id="rsp-export-from" type="number" min="0" step="1"></label>
            <label>To <input id="rsp-export-to" type="number" min="0" step="1"></label>
        </div>
        <div class="export-quick">
            <button type="button" data-q="this">This slide</button>
            <button type="button" data-q="module">This module</button>
            <button type="button" data-q="all">Whole deck</button>
        </div>
        <div class="export-mode" role="radiogroup" aria-label="Kind of PDF">
            <label><input type="radio" name="rsp-export-mode" value="exact" checked> <b>Exact</b> the slides as on screen</label>
            <label><input type="radio" name="rsp-export-mode" value="text"> <b>Text</b> lighter, selectable text</label>
        </div>
        <button class="export-go" type="submit"><span class="ms">download</span> Download the PDF</button>
        <button class="export-alt" type="button" data-q="print">or open the print view</button>
        <p class="export-hint">16:9, one slide per page, no margins. Exact: pictures of the slides, as on screen. Text: drawn by the print engine, text you can select and search. Needs Chrome, Chromium, Brave or Edge installed.</p>
    </form>`);
if (top && !OFF) {
    top.appendChild(exportOv);
    const form = exportOv.querySelector('form');
    const iFrom = exportOv.querySelector('#rsp-export-from');
    const iTo = exportOv.querySelector('#rsp-export-to');
    const clamp = (v) => Math.max(0, Math.min(slideCount, Math.round(Number(v) || 0)));
    const moduleRange = () => loadIndex().then((rows) => {
        let a = 0; let b = slideCount;
        for (const r of rows) {
            if (!r.m) continue;
            if (r.n <= slideNo) a = r.n; else { b = r.n - 1; break; }
        }
        return [a, b];
    });
    exportOv.querySelector('.export-quick').addEventListener('click', (ev) => {
        const q = ev.target.closest('button') && ev.target.closest('button').dataset.q;
        if (q === 'this') { iFrom.value = slideNo; iTo.value = slideNo; }
        if (q === 'all') { iFrom.value = 0; iTo.value = slideCount; }
        if (q === 'module') moduleRange().then(([a, b]) => { iFrom.value = a; iTo.value = b; });
    });
    const range = () => {
        let a = clamp(iFrom.value); let b = clamp(iTo.value);
        if (b < a) [a, b] = [b, a];
        return [a, b];
    };
    const hint = exportOv.querySelector('.export-hint');
    try {
        const saved = localStorage.getItem(`${NS}.export.mode`);
        const radio = saved && exportOv.querySelector(`input[name="rsp-export-mode"][value="${saved}"]`);
        if (radio) radio.checked = true;
    } catch { /* private window */ }
    const go = exportOv.querySelector('.export-go');
    const theme = () => (root.classList.contains('halloween') ? '&theme=halloween' : '');
    exportOv.querySelector('.export-alt').addEventListener('click', () => {
        const [a, b] = range();
        window.open(`/media/print.html?from=${a}&to=${b}&deck=${TRACK}${theme()}`, '_blank');
        exportOv._close();
    });
    // The PDF is made by headless Chrome through the presenter's server (respawn's PDF export) and
    // arrives as a download: no print dialog, no header or footer.
    form.addEventListener('submit', (ev) => {
        ev.preventDefault();
        if (go.disabled) return;
        const [a, b] = range();
        const n = b - a + 1;
        go.disabled = true;
        go.innerHTML = `<span class="ms">hourglass_top</span> Making ${n} slide${n > 1 ? 's' : ''}…`;
        const t0 = Date.now();
        const tick = setInterval(() => {
            hint.textContent = `Working… ${Math.round((Date.now() - t0) / 1000)} s (about ${Math.max(5, Math.round(n * 0.8))} s for ${n} slides). You can keep presenting: the file downloads when it is ready.`;
        }, 1000);
        const mode = (exportOv.querySelector('input[name="rsp-export-mode"]:checked') || { value: 'exact' }).value;
        try { localStorage.setItem(`${NS}.export.mode`, mode); } catch { /* private window */ }
        fetch(`/__pdf?from=${a}&to=${b}&mode=${mode}${theme()}`, { cache: 'no-store' })
            .then(async (r) => {
                if (!r.ok) throw new Error(await r.text());
                const cd = r.headers.get('Content-Disposition') || '';
                const m = /filename\*=UTF-8''([^;]+)/.exec(cd);
                const name = m ? decodeURIComponent(m[1]) : `AI4Dev slides ${a}-${b}.pdf`;
                const url = URL.createObjectURL(await r.blob());
                const link = document.createElement('a');
                link.href = url; link.download = name;
                document.body.appendChild(link); link.click(); link.remove();
                setTimeout(() => URL.revokeObjectURL(url), 10000);
                hint.textContent = `Downloaded: ${name}`;
            })
            .catch((e) => {
                const msg = String((e && e.message) || '');
                hint.textContent = msg.slice(0, 240) || 'The export failed: use the print view.';
            })
            .finally(() => {
                clearInterval(tick);
                go.disabled = false;
                go.innerHTML = '<span class="ms">download</span> Download the PDF';
            });
    });
    window.addEventListener('keydown', (ev) => {
        if (!exportOv.contains(ev.target)) return;
        const k = ev['ke' + 'y'];
        if (k === 'Escape') { ev.preventDefault(); exportOv.classList.remove('open'); document.activeElement.blur(); }
        if (k === 'Enter') { ev.preventDefault(); form.requestSubmit(); }
    }, true);
    exportOv.addEventListener('click', (ev) => { if (ev.target === exportOv) { exportOv.classList.remove('open'); document.activeElement.blur(); } });
    exportOv._close = () => { exportOv.classList.remove('open'); if (exportOv.contains(document.activeElement)) document.activeElement.blur(); };
    shortcuts.e = () => {
        closeOverlays();
        iFrom.value = slideNo; iTo.value = slideNo;
        exportOv.classList.add('open');
        setTimeout(() => iFrom.focus(), 20);
    };
}

// ------------------------------------------------------------------ audience toolbar
// Phones have no keyboard: the audience gets two buttons for search and notes.
if (AUD && top) {
    const bar = el('div', 'aud-tools', `
        <button type="button" data-a="search" title="Search the slides"><span class="ms">search</span></button>
        <button type="button" data-a="note" title="Add a note to this slide"><span class="ms">sticky_note_2</span></button>
        <button type="button" data-a="ask" title="Ask the trainer a question"><span class="ms">contact_support</span></button>`);
    top.appendChild(bar);
    bar.addEventListener('click', (ev) => {
        const b = ev.target.closest('button');
        if (!b) return;
        if (b.dataset.a === 'search') openSearch();
        if (b.dataset.a === 'note' && shortcuts.n) shortcuts.n();
        if (b.dataset.a === 'ask' && window.RESPAWN_ASK) window.RESPAWN_ASK();
    });
}

// ------------------------------------------------------------------ polls
// The presenter presses "q", writes a question and its answers, picks how many seconds the
// room has, and starts it. The poll lives in the audience server (the audience server): the
// audience copy shows it full screen with a countdown, and everybody sees the results
// when it closes. Slide changes do not interrupt it.
const POLL_API = AUD ? '' : `http://localhost:${AUDIENCE_PORT}`;
const pollFetch = (path, body) => fetch(`${POLL_API}${path}`, body === undefined
    ? { cache: 'no-store' }
    : { method: 'POST', headers: { 'Content-Type': 'text/plain' }, body: JSON.stringify(body), cache: 'no-store' })
    .then((r) => r.json());

const LETTERS = 'ABCDEFGH';
const pollResults = (p, mine) => {
    const counts = p.counts || [];
    const total = counts.reduce((a, b) => a + b, 0);
    const max = Math.max(1, ...counts);
    return `<ol class="poll-bars">${p.opts.map((o, i) => {
        const c = counts[i] || 0;
        const pct = total ? Math.round((100 * c) / total) : 0;
        const win = total && c === max ? ' win' : '';
        const me = mine === i ? ' mine' : '';
        return `<li class="${win}${me}"><span class="pb-l">${LETTERS[i]}</span>
            <span class="pb-main"><span class="pb-t">${escHtml(o)}</span>
            <span class="pb-track"><span class="pb-fill" style="width:${(100 * c) / max}%"></span></span></span>
            <span class="pb-n">${c}<small>${pct}%</small></span></li>`;
    }).join('')}</ol>`;
};
const pollClock = (left) => `<span class="poll-clock${left <= 5 ? ' hurry' : ''}">${Math.ceil(left)}<small>s</small></span>`;
// Redraw a poll card only when what it shows changes; the countdown ticks in place, so the
// buttons stay put under the pointer.
const pollPaint = (ov, sig, html, left) => {
    if (ov.dataset.sig !== sig) { ov.dataset.sig = sig; ov.innerHTML = html; }
    const c = ov.querySelector('.poll-clock');
    if (c) { c.innerHTML = `${Math.ceil(left)}<small>s</small>`; c.classList.toggle('hurry', left <= 5); }
};

// --- presenter
if (!AUD && !OFF && top) {
    const ov = el('div', 'ov poll-ov', '');
    top.appendChild(ov);
    let timer = 0;
    let mode = 'compose';
    const DISMISSED = `${NS}.poll.dismissed`;

    const compose = (prefill) => {
        mode = 'compose';
        clearInterval(timer);
        ov.dataset.sig = '';
        const opts = (prefill && prefill.opts) || ['', '', '', ''];
        ov.innerHTML = `
            <form class="poll-card poll-compose" aria-label="New poll">
                <h2>NEW POLL</h2>
                <label class="pc-q">Question<textarea id="rsp-poll-q" rows="2" placeholder="What does a train loss of 0.000 tell you?">${escHtml((prefill && prefill.q) || '')}</textarea></label>
                <div class="pc-presets">
                    <button type="button" data-p="yn">Yes / No</button>
                    <button type="button" data-p="abcd">A B C D</button>
                    <button type="button" data-p="scale">1 – 5</button>
                    <button type="button" data-p="add">+ answer</button>
                </div>
                <ol class="pc-opts">${opts.map((o, i) => `<li><span>${LETTERS[i]}</span><input type="text" id="rsp-poll-o${i}" value="${escHtml(o)}" placeholder="Answer ${LETTERS[i]}"></li>`).join('')}</ol>
                <div class="pc-time">
                    <label>Seconds to answer <input id="rsp-poll-s" type="number" min="5" max="600" step="1" value="${(prefill && prefill.seconds) || 30}"></label>
                    <span class="pc-quick"><button type="button" data-s="15">15</button><button type="button" data-s="30">30</button><button type="button" data-s="60">60</button><button type="button" data-s="120">120</button></span>
                </div>
                <button class="poll-go" type="submit"><span class="ms">how_to_vote</span> Start the poll</button>
                <p class="poll-msg" role="status"></p>
                <div class="poll-past"></div>
            </form>`;
        pastList();
        const form = ov.querySelector('form');
        const list = ov.querySelector('.pc-opts');
        const setOpts = (arr) => {
            list.innerHTML = arr.map((o, i) => `<li><span>${LETTERS[i]}</span><input type="text" id="rsp-poll-o${i}" value="${escHtml(o)}" placeholder="Answer ${LETTERS[i]}"></li>`).join('');
        };
        ov.querySelector('.pc-presets').addEventListener('click', (ev) => {
            const p = ev.target.closest('button') && ev.target.closest('button').dataset.p;
            const cur = [...list.querySelectorAll('input')].map((i) => i.value);
            if (p === 'yn') setOpts(['Yes', 'No']);
            if (p === 'abcd') setOpts(['A', 'B', 'C', 'D']);
            if (p === 'scale') setOpts(['1', '2', '3', '4', '5']);
            if (p === 'add' && cur.length < LETTERS.length) setOpts([...cur, '']);
        });
        ov.querySelector('.pc-quick').addEventListener('click', (ev) => {
            const s = ev.target.closest('button') && ev.target.closest('button').dataset.s;
            if (s) ov.querySelector('#rsp-poll-s').value = s;
        });
        form.addEventListener('submit', (ev) => {
            ev.preventDefault();
            const q = ov.querySelector('#rsp-poll-q').value.trim();
            const opts2 = [...list.querySelectorAll('input')].map((i) => i.value.trim()).filter(Boolean);
            const seconds = Number(ov.querySelector('#rsp-poll-s').value) || 30;
            const msg = ov.querySelector('.poll-msg');
            if (!q || opts2.length < 2) { msg.textContent = 'Write a question and at least two answers.'; return; }
            pollFetch('/__poll/start', { q, opts: opts2, seconds })
                .then((p) => { if (!p || !p.id) throw new Error(); store.set(DISMISSED, null); live(p); })
                .catch(() => { msg.textContent = 'The audience copy is not running: start the deck with respawn serve --audience.'; });
        });
        ov.classList.add('open');
        setTimeout(() => ov.querySelector('#rsp-poll-q').focus(), 30);
    };

    const live = (first) => {
        mode = 'live';
        clearInterval(timer);
        let p = first;
        ov.dataset.pid = p.id;
        const ends = Date.now() + p.left * 1000;
        const draw = () => {
            const left = Math.max(0, (ends - Date.now()) / 1000);
            const open = p.open && left > 0;
            pollPaint(ov, JSON.stringify([p.id, open, p.counts, p.total]), `
                <div class="poll-card poll-live${open ? '' : ' closed'}">
                    <div class="poll-head"><span class="poll-tag">${open ? 'POLL · LIVE' : 'POLL · RESULTS'}</span>${open ? pollClock(left) : ''}</div>
                    <h2 class="poll-q">${escHtml(p.q)}</h2>
                    ${pollResults(p)}
                    <div class="poll-foot"><span class="poll-total">${p.total} vote${p.total === 1 ? '' : 's'}</span>
                        <span class="poll-actions">
                            ${open ? '<button type="button" data-a="stop">Stop now</button>' : '<button type="button" data-a="again">Run it again</button><button type="button" data-a="new">New poll</button><button type="button" data-a="clear" class="danger">Clear results</button>'}
                            <button type="button" data-a="close">Close</button></span></div>
                </div>`, left);
        };
        ov.dataset.sig = '';
        draw();
        ov.classList.add('open');
        const clock = setInterval(() => { if (mode === 'live' && ov.classList.contains('open')) draw(); else clearInterval(clock); }, 250);
        timer = setInterval(() => {
            pollFetch('/__poll').then((np) => { if (np && np.id === p.id) p = np; draw(); }).catch(draw);
        }, 1000);
        ov.onclick = (ev) => {
            const a = ev.target.closest('button') && ev.target.closest('button').dataset.a;
            if (a === 'stop') pollFetch('/__poll/stop', {}).then((np) => { p = np; draw(); });
            if (a === 'close') close(p.id);
            if (a === 'new') compose();
            if (a === 'again') compose({ q: p.q, opts: p.opts });
            if (a === 'clear') pollFetch('/__poll/clear', { id: p.id }).then(() => compose()).catch(() => {});
        };
    };

    // Earlier polls under the form: results stay until you clear them (one, or all).
    const pastList = () => pollFetch('/__poll/history').then((rows) => {
        const box = ov.querySelector('.poll-past');
        if (!box) return;
        if (!rows.length) { box.innerHTML = ''; return; }
        box.innerHTML = `
            <div class="pp-head"><h3>Past polls</h3><button type="button" data-pp="all" class="danger">Clear all</button></div>
            <ol class="pp-list">${rows.map((r) => {
                const best = Math.max(0, ...r.counts);
                const win = r.total ? r.opts.filter((o, i) => r.counts[i] === best).join(' / ') : '';
                return `<li><span class="pp-q">${escHtml(r.q)}</span>
                    <span class="pp-m">${r.open ? 'live now · ' : ''}${r.at} · ${r.total} vote${r.total === 1 ? '' : 's'}${win ? ` · ${escHtml(win)}` : ''}</span>
                    <span class="pp-b"><button type="button" data-pp="show" data-id="${r.id}">Results</button><button type="button" data-pp="clear" data-id="${r.id}" class="danger">Clear</button></span></li>`;
            }).join('')}</ol>`;
        box.onclick = (ev) => {
            const b = ev.target.closest('button[data-pp]');
            if (!b) return;
            ev.preventDefault();
            const id = Number(b.dataset.id);
            if (b.dataset.pp === 'all') pollFetch('/__poll/clear', { all: true }).then(pastList);
            if (b.dataset.pp === 'clear') pollFetch('/__poll/clear', { id }).then(pastList);
            if (b.dataset.pp === 'show') { const r = rows.find((x) => x.id === id); if (r) past(r); }
        };
    }).catch(() => {});

    // One earlier poll's results, as they were when it closed.
    const past = (r) => {
        mode = 'past';
        clearInterval(timer);
        ov.dataset.sig = '';
        ov.innerHTML = `
            <div class="poll-card poll-live closed">
                <div class="poll-head"><span class="poll-tag">POLL · ${escHtml(r.at)}</span></div>
                <h2 class="poll-q">${escHtml(r.q)}</h2>
                ${pollResults(r)}
                <div class="poll-foot"><span class="poll-total">${r.total} vote${r.total === 1 ? '' : 's'}</span>
                    <span class="poll-actions"><button type="button" data-a="back">Back</button>
                    <button type="button" data-a="again">Run it again</button>
                    <button type="button" data-a="clear" class="danger">Clear results</button>
                    <button type="button" data-a="close">Close</button></span></div>
            </div>`;
        ov.classList.add('open');
        ov.onclick = (ev) => {
            const a = ev.target.closest('button') && ev.target.closest('button').dataset.a;
            if (a === 'back') { ov.onclick = null; compose(); }
            if (a === 'again') { ov.onclick = null; compose({ q: r.q, opts: r.opts }); }
            if (a === 'clear') pollFetch('/__poll/clear', { id: r.id }).then(() => { ov.onclick = null; compose(); });
            if (a === 'close') close();
        };
    };
    const close = (id) => {
        clearInterval(timer);
        if (ov.contains(document.activeElement)) document.activeElement.blur();
        ov.classList.remove('open');
        ov.onclick = null;
        if (id) store.set(DISMISSED, id);
    };
    ov._close = () => (mode === 'live' ? close(Number(ov.dataset.pid)) : close());

    window.addEventListener('keydown', (ev) => {
        if (!ov.contains(ev.target)) return;
        const k = ev['ke' + 'y'];
        if (k === 'Escape') { ev.preventDefault(); close(); }
        if (k === 'Enter' && (ev.metaKey || ev.ctrlKey)) { ev.preventDefault(); ov.querySelector('form') && ov.querySelector('form').requestSubmit(); }
    }, true);

    shortcuts.q = () => {
        if (ov.classList.contains('open')) { close(); return; }
        pollFetch('/__poll').then((p) => (p && p.open ? live(p) : compose())).catch(() => compose());
    };
    // A poll still running (or just closed and not dismissed) comes back on the next slide.
    pollFetch('/__poll').then((p) => {
        if (p && p.id && p.q && store.get(DISMISSED, 0) !== p.id && (p.open || p.total)) live(p);
    }).catch(() => {});
}

// --- audience
if (AUD && top) {
    const ov = el('div', 'ov poll-ov aud-poll', '');
    top.appendChild(ov);
    const VOTER = (() => {
        let v = store.get(`${NS}.voter`, null);
        if (!v) { v = Math.random().toString(36).slice(2) + Date.now().toString(36); store.set(`${NS}.voter`, v); }
        return v;
    })();
    const mineKey = (id) => `${NS}.poll.${id}`;
    let p = null;
    let ends = 0;
    const draw = () => {
        if (!p || !p.id || !p.q) { ov.classList.remove('open'); return; }
        const seen = store.get(`${NS}.poll.seen.${p.id}`, false);
        const left = Math.max(0, (ends - Date.now()) / 1000);
        const open = p.open && left > 0;
        if (!open && seen) { ov.classList.remove('open'); return; }
        const mine = store.get(mineKey(p.id), null);
        pollPaint(ov, JSON.stringify([p.id, open, mine, open ? 0 : p.counts, open ? 0 : p.total]), open ? `
            <div class="poll-card poll-vote">
                <div class="poll-head"><span class="poll-tag">POLL</span>${pollClock(left)}</div>
                <h2 class="poll-q">${escHtml(p.q)}</h2>
                <div class="poll-choices">${p.opts.map((o, i) => `<button type="button" data-c="${i}" class="${mine === i ? 'mine' : ''}"><span class="pb-l">${LETTERS[i]}</span>${escHtml(o)}</button>`).join('')}</div>
                <p class="poll-msg">${mine === null ? 'Tap an answer.' : 'Answer sent. You can change it until the time is up.'}</p>
            </div>` : `
            <div class="poll-card poll-live closed">
                <div class="poll-head"><span class="poll-tag">POLL · RESULTS</span></div>
                <h2 class="poll-q">${escHtml(p.q)}</h2>
                ${p.counts ? pollResults(p, mine) : '<p class="poll-msg">Counting…</p>'}
                <div class="poll-foot"><span class="poll-total">${p.total} vote${p.total === 1 ? '' : 's'}</span>
                    <span class="poll-actions"><button type="button" data-a="close">Close</button></span></div>
            </div>`, left);
        ov.classList.add('open');
    };
    ov.addEventListener('click', (ev) => {
        const b = ev.target.closest('button');
        if (!b || !p) return;
        if (b.dataset.a === 'close') { store.set(`${NS}.poll.seen.${p.id}`, true); draw(); return; }
        if (b.dataset.c !== undefined) {
            const choice = Number(b.dataset.c);
            pollFetch('/__poll/vote', { id: p.id, choice, voter: VOTER })
                .then((r) => { if (r && r.ok) { store.set(mineKey(p.id), choice); draw(); } })
                .catch(() => {});
        }
    });
    ov._close = () => { if (p) store.set(`${NS}.poll.seen.${p.id}`, true); ov.classList.remove('open'); };
    const tick = () => pollFetch('/__poll')
        .then((np) => { p = np; ends = Date.now() + (np.left || 0) * 1000; draw(); })
        .catch(() => {})
        .finally(() => setTimeout(tick, 1000));
    tick();
    setInterval(() => { if (p && p.open) draw(); }, 250);
}

// ------------------------------------------------------------------ exercise catalog
// Recap and conclusion exercises are not in the slide sequence: "x" opens the catalog,
// pick one and it takes over the screen (← → for its steps, Esc to go back to the slide).
// One catalog for both decks (assets/media/exercises.json, built with the decks): the
// presenter picks the exercise; a chip says when it comes from the other deck.
if (top && !OFF) {
    let exercises = null;
    const loadExercises = () => exercises || (exercises = fetch('/media/exercises.json', { cache: 'no-store' })
        .then((r) => (r.ok ? r.json() : [])).catch(() => []));

    const cat = el('div', 'ov catalog-ov', `
        <div class="catalog-card" role="dialog" aria-label="Exercise catalog">
            <div class="catalog-head"><h2>EXERCISES</h2>
                <div class="catalog-tabs" role="tablist">
                    <button type="button" data-k="all" class="on">All</button>
                    <button type="button" data-k="generic">Generic</button>
                    <button type="button" data-k="module">By module</button>
                </div></div>
            <input id="rsp-catalog-q" type="search" placeholder="Filter…" autocomplete="off" spellcheck="false">
            <ol class="catalog-list"></ol>
            <p class="search-hint">↑ ↓ to move · Enter to run · Esc to close</p>
        </div>`);
    const stage = el('div', 'ex-stage', '');
    top.appendChild(cat);
    top.appendChild(stage);
    const qIn = cat.querySelector('input');
    const list = cat.querySelector('.catalog-list');
    let kind = 'all';
    let shown = [];
    let sel = 0;

    const render = () => loadExercises().then((all) => {
        const words = fold(qIn.value).split(/\s+/).filter(Boolean);
        shown = all.filter((x) => (kind === 'all' || (kind === 'generic') === Boolean(x.generic))
            && words.every((w) => fold(`${x.title} ${x.label} ${x.module}`).includes(w)));
        sel = Math.min(sel, Math.max(0, shown.length - 1));
        list.innerHTML = shown.length ? shown.map((x, i) => `
            <li class="${i === sel ? 'sel' : ''}" data-i="${i}">
                <span class="cx-kind${x.generic ? ' generic' : ''}">${escHtml(x.label)}</span>
                <span class="cx-main"><span class="cx-t">${escHtml(x.title)}</span>
                <span class="cx-m">${escHtml(x.generic ? 'Works after any module' : x.module.replace(/^Module \d+ · /, ''))}${x.steps.length > 1 ? ` · ${x.steps.length} steps` : ''}${(x.decks || []).length === 1 ? ` <b class="cx-deck">${escHtml((CFG.labels || {})[x.decks[0]] || x.decks[0].toUpperCase())}</b>` : ''}</span></span>
                <span class="cx-time">${escHtml(x.timer)}</span>
            </li>`).join('') : '<li class="cx-empty">No exercise matches.</li>';
        const cur = list.querySelector('.sel');
        if (cur) cur.scrollIntoView({ block: 'nearest' });
    });

    // --- running an exercise
    // The speaker-notes window (s) shows the exercise's notes while it is on screen.
    const emitNotes = (title, notes) => {
        try {
            const ch = new BroadcastChannel('demoit_nav');
            ch.postMessage({ currentSlideId: slideNo, stepCount: slideCount, currentSlideTitle: title, speakerNotes: notes });
            ch.close();
        } catch { /* no notes window */ }
    };
    let running = null;
    let step = 0;
    // The audience copy shows the same exercise and step (the audience server); no server, no harm.
    const tellRoom = (id, n) => fetch(`http://localhost:${AUDIENCE_PORT}/__exercise/set?id=${encodeURIComponent(id || '')}&step=${n || 0}`,
        { mode: 'no-cors', cache: 'no-store' }).catch(() => {});
    const show = () => {
        const x = running;
        tellRoom(x.id, step);
        stage.innerHTML = `${x.steps[step]}
            <div class="ex-bar"><span class="ex-tag">EXERCISE${x.steps.length > 1 ? ` · ${step + 1}/${x.steps.length}` : ''}</span>
            <span class="ex-help">${x.steps.length > 1 ? '← → steps · ' : ''}Esc back to the slide</span></div>`;
        for (const c of stage.querySelectorAll(STAGGER.join(','))) {
            c.setAttribute('data-stagger', '');
            [...c.children].forEach((child, i) => child.style.setProperty('--i', i));
        }
        stage.querySelectorAll('.timer').forEach(bindBadge);
        const n = x.notes[step] || '';
        emitNotes(`Exercise · ${escHtml(x.title)}`, n ? `<p>${escHtml(n).replace(/\n/g, '<br>')}</p>` : '');
    };
    const run = (x) => {
        running = x; step = 0;
        cat.classList.remove('open');
        qIn.blur();
        top.classList.add('exercise-on');
        show();
    };
    const stop = () => {
        running = null;
        tellRoom('', 0);
        top.classList.remove('exercise-on');
        stage.innerHTML = '';
        const sn = document.querySelector('speaker-notes');
        const h1 = document.querySelector('section.slide h1');
        emitNotes(h1 ? h1.innerHTML : '', sn ? sn.innerHTML : '');
    };
    stage._close = stop;

    window.addEventListener('keydown', (ev) => {
        const k = ev['ke' + 'y'];
        if (ev.target === qIn) {
            if (k === 'ArrowDown') { ev.preventDefault(); sel = Math.min(sel + 1, shown.length - 1); render(); }
            else if (k === 'ArrowUp') { ev.preventDefault(); sel = Math.max(sel - 1, 0); render(); }
            else if (k === 'Enter') { ev.preventDefault(); if (shown[sel]) run(shown[sel]); }
            else if (k === 'Escape') { ev.preventDefault(); cat.classList.remove('open'); qIn.blur(); }
            return;
        }
        if (!running || typing(ev.target, k) || overlayOpen()) return;
        // While an exercise is on screen the arrows move through its steps, not the deck.
        if (['ArrowRight', 'ArrowLeft', ' ', 'PageDown', 'PageUp', 'Escape'].includes(k)) {
            ev.preventDefault(); ev.stopPropagation(); ev.stopImmediatePropagation();
            if (k === 'Escape') { stop(); return; }
            const fwd = k === 'ArrowRight' || k === ' ' || k === 'PageDown';
            const next = step + (fwd ? 1 : -1);
            if (next >= 0 && next < running.steps.length) { step = next; show(); }
        }
    }, true);

    cat.querySelector('.catalog-tabs').addEventListener('click', (ev) => {
        const b = ev.target.closest('button');
        if (!b) return;
        kind = b.dataset.k; sel = 0;
        cat.querySelectorAll('.catalog-tabs button').forEach((x) => x.classList.toggle('on', x === b));
        render();
        qIn.focus();
    });
    qIn.addEventListener('input', () => { sel = 0; render(); });
    list.addEventListener('click', (ev) => { const li = ev.target.closest('li[data-i]'); if (li) run(shown[Number(li.dataset.i)]); });
    cat.addEventListener('click', (ev) => { if (ev.target === cat) { cat.classList.remove('open'); qIn.blur(); } });
    cat._close = () => { cat.classList.remove('open'); qIn.blur(); };

    shortcuts.x = () => {
        if (cat.classList.contains('open')) { cat._close(); return; }
        closeOverlays();
        qIn.value = '';
        kind = 'all'; sel = 0;
        cat.querySelectorAll('.catalog-tabs button').forEach((b) => b.classList.toggle('on', b.dataset.k === kind));
        cat.classList.add('open');
        render();
        setTimeout(() => qIn.focus(), 20);
    };
}

// ------------------------------------------------------------------ join screen
// "j" shows the QR code (and the short code) the room uses to join the audience copy.
// The session code comes from the audience server; with -tunnel the link is the public one
// (live.ai4dev.dev), otherwise this computer's address on the local network.
if (!AUD && !OFF && top) {
    const ov = el('div', 'ov join-ov', '');
    top.appendChild(ov);
    let timer = 0;
    let qrReady = null;
    const loadQr = () => qrReady || (qrReady = new Promise((ok, ko) => {
        const sc = document.createElement('script');
        sc.src = '/js/qrcode.js';
        sc.onload = () => ok(window.qrcode);
        sc.onerror = ko;
        document.head.appendChild(sc);
    }));
    const qrSvg = (qrcode, text) => {
        const qr = qrcode(0, 'M');
        qr.addData(text);
        qr.make();
        const n = qr.getModuleCount();
        let d = '';
        for (let r = 0; r < n; r++) for (let c = 0; c < n; c++) if (qr.isDark(r, c)) d += `M${c + 4} ${r + 4}h1v1h-1z`;
        return `<svg viewBox="0 0 ${n + 8} ${n + 8}" shape-rendering="crispEdges" role="img" aria-label="QR code"><rect width="100%" height="100%" fill="#fff"/><path d="${d}" fill="#000"/></svg>`;
    };
    const close = () => { clearInterval(timer); ov.classList.remove('open'); };
    ov._close = close;
    ov.addEventListener('click', close);
    const count = () => fetch(`http://localhost:${AUDIENCE_PORT}/__session`, { cache: 'no-store' })
        .then((r) => r.json())
        .then((d) => { const v = ov.querySelector('.join-viewers'); if (v) v.textContent = `${d.viewers} connected`; })
        .catch(() => {});
    shortcuts.j = () => {
        if (ov.classList.contains('open')) { close(); return; }
        closeOverlays();
        Promise.all([fetch(`http://localhost:${AUDIENCE_PORT}/__session`, { cache: 'no-store' }).then((r) => r.json()), loadQr()])
            .then(([d, qrcode]) => {
                const host = d.base.replace(/^https?:\/\//, '');
                ov.innerHTML = `
                    <div class="join-card">
                        <div class="join-qr">${qrSvg(qrcode, d.url)}</div>
                        <div class="join-text">
                            <span class="join-tag">JOIN THE SESSION</span>
                            <p class="join-step">Scan the code, or open</p>
                            <p class="join-host">${escHtml(host)}</p>
                            <p class="join-step">and type</p>
                            <p class="join-code">${escHtml(d.code)}</p>
                            <p class="join-viewers">${d.viewers} connected</p>
                        </div>
                    </div>`;
                ov.classList.add('open');
                clearInterval(timer);
                timer = setInterval(count, 2000);
            })
            .catch(() => {
                ov.innerHTML = '<div class="join-card"><p class="join-step">The audience copy is not running: start the deck with <b>respawn serve --audience</b> or <b>--tunnel</b>.</p></div>';
                ov.classList.add('open');
            });
    };
}

// ------------------------------------------------------------------ exercise, audience side
// When the presenter runs an exercise from the catalog, the audience copy shows the same
// exercise and step over whatever slide each person is on, and nothing moves it but the
// presenter: no keys, no clicks through to the slide, until the presenter closes it.
if (AUD && top) {
    let exercises = null;
    const load = () => exercises || (exercises = fetch('/media/exercises.json', { cache: 'no-store' })
        .then((r) => (r.ok ? r.json() : [])).catch(() => []));
    const stage = el('div', 'ex-stage', '');
    top.appendChild(stage);
    let shown = '';
    window.RESPAWN_SHOW_EXERCISE = (ex) => {
        const key = ex && ex.id ? `${ex.id}#${ex.step}` : '';
        if (key === shown) return;
        shown = key;
        if (!key) { top.classList.remove('exercise-on'); stage.innerHTML = ''; return; }
        load().then((all) => {
            const x = all.find((e) => e.id === ex.id);
            if (!x || shown !== key) return;
            const n = Math.max(0, Math.min(ex.step || 0, x.steps.length - 1));
            closeOverlays();
            stage.innerHTML = `${x.steps[n]}
                <div class="ex-bar"><span class="ex-tag">EXERCISE${x.steps.length > 1 ? ` · ${n + 1}/${x.steps.length}` : ''}</span></div>`;
            for (const c of stage.querySelectorAll(STAGGER.join(','))) {
                c.setAttribute('data-stagger', '');
                [...c.children].forEach((child, i) => child.style.setProperty('--i', i));
            }
            top.classList.add('exercise-on');
        });
    };
    // While it is on screen nothing reaches the deck: the presenter leads.
    window.addEventListener('keydown', (ev) => {
        if (!top.classList.contains('exercise-on')) return;
        ev.preventDefault(); ev.stopPropagation(); ev.stopImmediatePropagation();
    }, true);
}

// ------------------------------------------------------------------ hold the room
// "l" stops the audience copy from following: the room stays on the slide it is on (and
// cannot go past it) while the presenter browses anywhere. "l" again, or a click on the
// chip, releases it and brings the room to the presenter's current slide.
// The state lives in the audience server, so it survives the presenter's page changes.
if (!AUD && !OFF && top) {
    const api = `http://localhost:${AUDIENCE_PORT}`;
    const chip = el('button', 'hold-chip', '');
    chip.type = 'button';
    chip.hidden = true;
    top.appendChild(chip);
    const paint = (d) => {
        const held = Boolean(d && d.held);
        chip.hidden = !held;
        if (held) chip.innerHTML = `<span class="ms">pause_circle</span>ROOM HELD ON ${d.n} · L TO RELEASE`;
    };
    const state = () => fetch(`${api}/__live`, { cache: 'no-store' }).then((r) => r.json());
    const hold = (on) => fetch(`${api}/__live/hold?on=${on ? 1 : 0}&n=${slideNo}`, { cache: 'no-store' })
        .then((r) => r.json()).then(paint).catch(() => {});
    state().then(paint).catch(() => {});
    shortcuts.l = () => state().then((d) => hold(!d.held)).catch(() => {});
    chip.addEventListener('click', () => hold(false));
}

// ------------------------------------------------------------------ halloween
// "w" turns the deck into its Halloween edition and back: pumpkin, slime and witch-purple
// instead of the usual palette, a moon, bats, a cobweb and a spider, same comic ink.
// Kept in the browser, and sent to the audience copy so the room gets it too.
const THEME = `${NS}.theme`;
const HW_DECO = `
<svg class="hw-moon" viewBox="0 0 100 100" aria-hidden="true">
  <circle cx="50" cy="50" r="46" fill="#ffcf6b" stroke="#000" stroke-width="3"/>
  <circle cx="34" cy="38" r="8" fill="#f2a93b"/><circle cx="62" cy="60" r="11" fill="#f2a93b"/><circle cx="60" cy="28" r="5" fill="#f2a93b"/>
  <path d="M18 70 Q 40 92 74 82" fill="none" stroke="#e08a1e" stroke-width="4" stroke-linecap="round"/>
</svg>
<svg class="hw-web" viewBox="0 0 100 100" aria-hidden="true">
  <g fill="none" stroke="#efe2c4" stroke-width="1.2" stroke-linecap="round">
    <path d="M0 0 L100 8 M0 0 L92 40 M0 0 L70 72 M0 0 L38 94 M0 0 L8 100"/>
    <path d="M22 2 Q 21 9 20 9 Q 17 15 16 18 Q 10 21 8 22 Q 3 22 2 22"/>
    <path d="M46 4 Q 43 16 41 18 Q 34 30 32 34 Q 22 39 18 42 Q 9 44 4 45"/>
    <path d="M70 6 Q 66 22 63 28 Q 52 46 48 51 Q 33 60 27 63 Q 13 66 6 68"/>
    <path d="M94 8 Q 88 30 84 38 Q 70 60 64 66 Q 44 80 36 84 Q 18 89 8 92"/>
  </g>
</svg>
<div class="hw-spider" aria-hidden="true"><i></i>
  <svg viewBox="0 0 60 50"><g stroke="#000" stroke-width="3" fill="none" stroke-linecap="round">
    <path d="M22 22 L6 10 M22 26 L3 24 M22 30 L5 40 M24 33 L12 48 M38 22 L54 10 M38 26 L57 24 M38 30 L55 40 M36 33 L48 48"/></g>
    <ellipse cx="30" cy="28" rx="11" ry="13" fill="#1a0d12" stroke="#000" stroke-width="3"/>
    <circle cx="30" cy="13" r="7" fill="#1a0d12" stroke="#000" stroke-width="3"/>
    <circle cx="27" cy="12" r="1.8" fill="#8cff3a"/><circle cx="33" cy="12" r="1.8" fill="#8cff3a"/>
  </svg>
</div>
${[0, 1, 2, 3].map((i) => `<svg class="hw-bat b${i}" viewBox="0 0 64 28" aria-hidden="true"><path d="M32 10 C 30 4 27 3 26 6 C 22 2 14 1 2 6 C 9 8 11 12 10 16 C 14 13 18 14 20 18 C 23 15 27 16 32 22 C 37 16 41 15 44 18 C 46 14 50 13 54 16 C 53 12 55 8 62 6 C 50 1 42 2 38 6 C 37 3 34 4 32 10 Z" fill="#0b0508" stroke="#000" stroke-width="2"/><circle cx="29.5" cy="11" r="1.1" fill="#ff9a1f"/><circle cx="34.5" cy="11" r="1.1" fill="#ff9a1f"/></svg>`).join('')}
<svg class="hw-pumpkin" viewBox="0 0 100 90" aria-hidden="true">
  <path d="M50 16 C 52 8 58 4 64 4" fill="none" stroke="#3f7a1c" stroke-width="6" stroke-linecap="round"/>
  <g stroke="#000" stroke-width="3.5">
    <ellipse cx="30" cy="52" rx="24" ry="32" fill="#e8700d"/><ellipse cx="70" cy="52" rx="24" ry="32" fill="#e8700d"/>
    <ellipse cx="50" cy="52" rx="22" ry="35" fill="#ff8a00"/>
  </g>
  <path d="M30 42 L40 32 L44 46 Z M70 42 L60 32 L56 46 Z" fill="#ffe14d" stroke="#000" stroke-width="2.5" stroke-linejoin="round"/>
  <path d="M24 60 Q 50 84 76 60 L 68 62 L 62 70 L 56 63 L 50 71 L 44 63 L 38 70 L 32 62 Z" fill="#ffe14d" stroke="#000" stroke-width="2.5" stroke-linejoin="round"/>
</svg>`;

const applyTheme = (name) => {
    const on = name === 'halloween';
    root.classList.toggle('halloween', on);
    if (!top) return;
    let deco = top.querySelector('.hw-deco');
    if (on && !deco) { deco = el('div', 'hw-deco', HW_DECO); top.appendChild(deco); }
    if (!on && deco) deco.remove();
};
// A theremin glide, for the switch: an original synth, nothing sampled.
const spooky = () => {
    if (!soundOn()) return;
    try {
        const ctx = new (window.AudioContext || window.webkitAudioContext)();
        const t0 = ctx.currentTime + 0.02;
        const o = ctx.createOscillator();
        const g = ctx.createGain();
        const lfo = ctx.createOscillator();
        const lg = ctx.createGain();
        o.type = 'sine';
        o.frequency.setValueAtTime(330, t0);
        o.frequency.exponentialRampToValueAtTime(880, t0 + 0.6);
        o.frequency.exponentialRampToValueAtTime(220, t0 + 1.6);
        lfo.frequency.value = 6;
        lg.gain.value = 18;
        lfo.connect(lg).connect(o.frequency);
        g.gain.setValueAtTime(0.0001, t0);
        g.gain.exponentialRampToValueAtTime(0.18, t0 + 0.15);
        g.gain.exponentialRampToValueAtTime(0.0001, t0 + 1.8);
        o.connect(g).connect(ctx.destination);
        o.start(t0); lfo.start(t0);
        o.stop(t0 + 1.9); lfo.stop(t0 + 1.9);
    } catch { /* no audio */ }
};
const shareTheme = (name) => fetch(`http://localhost:${AUDIENCE_PORT}/__theme/set?name=${encodeURIComponent(name || '')}`,
    { mode: 'no-cors', cache: 'no-store' }).catch(() => {});

// ?theme= wins (the PDF export runs in a browser that has no stored preference)
if (!AUD) applyTheme(params.has('theme') ? params.get('theme') : store.get(THEME, ''));
if (!AUD && !OFF && top) {
    shareTheme(store.get(THEME, ''));
    shortcuts.w = () => {
        const next = store.get(THEME, '') === 'halloween' ? '' : 'halloween';
        store.set(THEME, next);
        applyTheme(next);
        shareTheme(next);
        if (next) spooky();
    };
}
// The audience copy follows the presenter's theme (from /__live, see the audience section).
window.RESPAWN_THEME = applyTheme;

// ------------------------------------------------------------------ questions from the room
// The audience copy has a "?" button: a question goes to the audience server. The presenter gets
// a notification in the corner; a click (or "a") opens the list, and a question picked from
// it opens over the slide as a big sticky note, for the room as well. "Answered" puts it away.
const qaNote = (q, presenter) => `
    <div class="qa-note" role="dialog" aria-label="Question from the room">
        <div class="qa-tag"><span class="ms">contact_support</span>QUESTION FROM THE ROOM</div>
        <p class="qa-text">${escHtml(q.text)}</p>
        <div class="qa-meta">${q.name ? escHtml(q.name) + ' · ' : ''}slide ${q.slide}</div>
        ${presenter ? '<div class="qa-act"><button type="button" data-q="done"><span class="ms">check</span>Answered <small>Enter</small></button><button type="button" data-q="hide">Later <small>Esc</small></button></div>' : ''}
    </div>`;

if (!AUD && !OFF && top) {
    const api = `http://localhost:${AUDIENCE_PORT}`;
    const post = (body) => fetch(`${api}/__questions/act`, {
        method: 'POST', cache: 'no-store', headers: { 'Content-Type': 'text/plain' }, body: JSON.stringify(body),
    }).then((r) => r.json());
    const bell = el('button', 'qa-bell', '');
    bell.type = 'button';
    bell.hidden = true;
    const panel = el('div', 'ov qa-ov', '');
    const stage = el('div', 'qa-stage', '');
    top.appendChild(bell); top.appendChild(panel); top.appendChild(stage);
    let data = { items: [], showing: null };

    let staged = '';
    const paintStage = () => {
        const q = data.items.find((x) => x.id === data.showing);
        const sig = q ? JSON.stringify([q.id, q.text, q.name, q.slide]) : '';
        if (sig === staged) return;   // unchanged: leave it alone, or its pop-in replays
        staged = sig;
        stage.innerHTML = q ? qaNote(q, true) : '';
        stage.classList.toggle('on', Boolean(q));
    };
    const paintBell = () => {
        const fresh = data.items.filter((q) => !q.seen).length;
        const open = data.items.filter((q) => !q.done).length;
        bell.hidden = !open;
        bell.classList.toggle('fresh', fresh > 0);
        bell.innerHTML = `<span class="ms">contact_support</span>${fresh ? `${fresh} NEW` : open}<small>${fresh ? `question${fresh > 1 ? 's' : ''}` : `open question${open > 1 ? 's' : ''}`}</small>`;
    };
    const paintPanel = () => {
        const rows = [...data.items].reverse();
        panel.innerHTML = `
            <div class="qa-card">
                <div class="qa-head"><h2>QUESTIONS</h2>
                    ${rows.length ? '<button type="button" data-q="clear" class="danger">Clear all</button>' : ''}</div>
                ${rows.length ? `<ol class="qa-list">${rows.map((q) => `
                    <li class="${q.done ? 'done' : ''}${q.seen ? '' : ' fresh'}" data-id="${q.id}">
                        <button type="button" class="qa-open" data-q="show" data-id="${q.id}">
                            <span class="qa-t">${escHtml(q.text)}</span>
                            <span class="qa-m">${q.at} · ${q.name ? escHtml(q.name) + ' · ' : ''}slide ${q.slide}${q.done ? ' · answered' : ''}</span>
                        </button>
                        <span class="qa-b">
                            <button type="button" data-q="${q.done ? 'undone' : 'done'}" data-id="${q.id}" title="${q.done ? 'Not answered yet' : 'Mark as answered'}"><span class="ms">${q.done ? 'undo' : 'check'}</span></button>
                            <button type="button" data-q="delete" data-id="${q.id}" class="danger" title="Delete"><span class="ms">delete</span></button>
                        </span></li>`).join('')}</ol>`
                    : '<p class="qa-empty">No questions yet. The room asks with the <b>?</b> button on their copy of the slides.</p>'}
                <p class="search-hint">Click a question to put it over the slide · Esc to close</p>
            </div>`;
    };
    const refresh = () => fetch(`${api}/__questions`, { cache: 'no-store' }).then((r) => r.json())
        .then((d) => { data = d; paintBell(); paintStage(); if (panel.classList.contains('open')) paintPanel(); })
        .catch(() => {});
    const act = (body) => post(body).then((d) => { data = d; paintBell(); paintStage(); if (panel.classList.contains('open')) paintPanel(); }).catch(() => {});

    const openPanel = () => {
        closeOverlays();
        paintPanel();
        panel.classList.add('open');
        if (data.items.some((q) => !q.seen)) act({ act: 'seen_all' });
    };
    panel._close = () => panel.classList.remove('open');
    bell.addEventListener('click', openPanel);
    panel.addEventListener('click', (ev) => {
        const b = ev.target.closest('button[data-q]');
        if (!b) { if (ev.target === panel) panel._close(); return; }
        const id = Number(b.dataset.id);
        if (b.dataset.q === 'show') { panel._close(); act({ act: 'show', id }); }
        else act({ act: b.dataset.q, id });
    });
    stage.addEventListener('click', (ev) => {
        const b = ev.target.closest('button[data-q]');
        if (!b) return;
        if (b.dataset.q === 'done') act({ act: 'done', id: data.showing });
        if (b.dataset.q === 'hide') act({ act: 'hide' });
    });
    window.addEventListener('keydown', (ev) => {
        if (!stage.classList.contains('on') || typing(ev.target, ev['ke' + 'y'])) return;
        const k = ev['ke' + 'y'];
        if (panel.classList.contains('open')) return;
        if (k === 'a') return;   // the list can still be opened
        ev.preventDefault(); ev.stopPropagation(); ev.stopImmediatePropagation();
        if (k === 'Escape') act({ act: 'hide' });
        if (k === 'Enter') act({ act: 'done', id: data.showing });
    }, true);
    shortcuts.a = () => (panel.classList.contains('open') ? panel._close() : openPanel());
    refresh();
    setInterval(refresh, 3000);
}

if (AUD && top) {
    // Ask: a small form over the slide; the answer comes in the room.
    const ov = el('div', 'ov ask-ov', `
        <form class="ask-card">
            <h2>ASK THE TRAINER</h2>
            <textarea id="rsp-ask-q" rows="4" maxlength="400" placeholder="Your question…"></textarea>
            <input id="rsp-ask-n" type="text" maxlength="40" placeholder="Your name (optional)" autocomplete="name">
            <div class="ask-row"><button type="submit" class="poll-go"><span class="ms">send</span> Send</button>
                <button type="button" data-a="cancel">Cancel</button></div>
            <p class="poll-msg" role="status"></p>
        </form>`);
    top.appendChild(ov);
    const form = ov.querySelector('form');
    const txt = ov.querySelector('#rsp-ask-q');
    const name = ov.querySelector('#rsp-ask-n');
    const msg = ov.querySelector('.poll-msg');
    try { name.value = store.get(`${NS}.ask.name`, '') || ''; } catch { /* no storage */ }
    const close = () => { if (ov.contains(document.activeElement)) document.activeElement.blur(); ov.classList.remove('open'); };
    ov._close = close;
    ov.addEventListener('click', (ev) => { if (ev.target === ov || (ev.target.closest('button') && ev.target.closest('button').dataset.a === 'cancel')) close(); });
    ov.addEventListener('keydown', (ev) => { if (ev['ke' + 'y'] === 'Escape') { ev.preventDefault(); close(); } });
    form.addEventListener('submit', (ev) => {
        ev.preventDefault();
        const text = txt.value.trim();
        if (!text) { msg.textContent = 'Write your question first.'; return; }
        store.set(`${NS}.ask.name`, name.value.trim());
        fetch('/__ask', { method: 'POST', cache: 'no-store', headers: { 'Content-Type': 'text/plain' },
            body: JSON.stringify({ text, name: name.value.trim() }) })
            .then((r) => r.json().then((d) => ({ ok: r.ok, d })))
            .then(({ ok, d }) => {
                if (ok && d.ok) { txt.value = ''; msg.textContent = 'Sent. The trainer will pick it up.'; setTimeout(close, 1200); }
                else msg.textContent = d && d.wait ? 'Wait a few seconds before the next question.' : 'Not sent. Try again.';
            })
            .catch(() => { msg.textContent = 'Not sent. Try again.'; });
    });
    window.RESPAWN_ASK = () => {
        closeOverlays();
        msg.textContent = '';
        ov.classList.add('open');
        setTimeout(() => txt.focus(), 30);
    };
    // The question the presenter has opened, over the slide (read-only).
    const stage = el('div', 'qa-stage', '');
    top.appendChild(stage);
    let shown = '';
    window.addEventListener('keydown', (ev) => {
        if (!stage.classList.contains('on') || typing(ev.target, ev['ke' + 'y'])) return;
        ev.preventDefault(); ev.stopPropagation(); ev.stopImmediatePropagation();
    }, true);
    window.RESPAWN_QUESTION = (q) => {
        const sig = q ? JSON.stringify(q) : '';
        if (sig === shown) return;
        shown = sig;
        stage.innerHTML = q ? qaNote(q, false) : '';
        stage.classList.toggle('on', Boolean(q));
    };
}

// ------------------------------------------------------------------ live quiz
// A "::: quiz" slide is a question for the whole room, game-show style. On the presenter's
// screen: Enter shows the options and starts the clock, Enter again closes it early, and
// once it is over the right answer lights up with how many picked each option and who was
// fastest; Enter then shows the leaderboard (k at any time), on the phones too.
// The phones (audience copy) ask for a player name, show the options as big buttons for
// the question's seconds, then the result: points for a right answer (more the faster),
// minus points for a wrong one, and for no answer once the two jokers are gone.
// The game, its scores and the answers live in the audience server, never in the slides.
const QUIZ_COLORS = ['yellow', 'cyan', 'magenta', 'green', 'orange', 'purple', 'blue', 'pink'];
const quizNum = (n) => (n > 0 ? `+${n.toLocaleString()}` : n.toLocaleString());
const quizBoard = (rows, me) => `<ol class="qb-rows">${rows.map((r) => `
    <li class="${r.me ? 'me' : ''}"><span class="qb-rank">${r.rank}</span><span class="qb-name">${escHtml(r.name)}</span>
        <span class="qb-delta ${r.delta > 0 ? 'up' : r.delta < 0 ? 'down' : ''}">${r.delta ? quizNum(r.delta) : ''}</span>
        <span class="qb-score">${r.score.toLocaleString()}</span></li>`).join('')}</ol>${me || ''}`;

// --- presenter
if (!AUD && !OFF && top) {
    const boardOv = el('div', 'ov quiz-board-ov', '');
    top.appendChild(boardOv);
    let resetArmed = false;
    const drawBoard = (d) => {
        const rows = (d.leaders || []).slice(0, 10);
        boardOv.innerHTML = `<div class="qb-card">
            <div class="qb-head"><span class="qb-tag">LEADERBOARD</span><span class="qb-count">${d.players || 0} player${d.players === 1 ? '' : 's'}</span></div>
            ${rows.length ? quizBoard(rows) : '<p class="qb-empty">Nobody is playing yet: the room joins with the QR code (j) and picks a name on the first question.</p>'}
            <div class="qb-foot"><button type="button" data-a="reset">${resetArmed ? 'Sure? Click again to reset the game' : 'Reset the game'}</button>
                <span>Esc or k to close</span></div></div>`;
    };
    const openBoard = () => pollFetch('/__quiz/board', { on: true })
        .then((d) => { resetArmed = false; drawBoard(d); boardOv.classList.add('open'); })
        .catch(() => toast('<span class="ms">leaderboard</span> START WITH respawn serve --audience'));
    boardOv._close = () => { boardOv.classList.remove('open'); pollFetch('/__quiz/board', { on: false }).catch(() => {}); };
    boardOv.addEventListener('click', (ev) => {
        const b = ev.target.closest('button');
        if (!b || b.dataset.a !== 'reset') return;
        if (!resetArmed) { resetArmed = true; pollFetch('/__quiz').then(drawBoard).catch(() => {}); return; }
        pollFetch('/__quiz/reset', {}).then((d) => { resetArmed = false; drawBoard(d); toast('<span class="ms">restart_alt</span> NEW GAME'); }).catch(() => {});
    });
    boardOv._keys = { k: () => boardOv._close(), Enter: () => boardOv._close() };
    shortcuts.k = () => (boardOv.classList.contains('open') ? boardOv._close() : openBoard());

    const card = top.querySelector('.quiz[data-quiz]');
    if (card) {
        const id = card.dataset.quiz;
        card.querySelectorAll('.quiz-opts li').forEach((li, i) => { li.dataset.c = QUIZ_COLORS[i % QUIZ_COLORS.length]; });
        const hud = el('div', 'quiz-hud', '');
        card.appendChild(hud);
        let phase = 'ready', end = 0, timer = 0, last = null;
        const paintClock = () => {
            const c = hud.querySelector('.qh-clock');
            if (!c || phase !== 'open') return;
            const left = Math.max(0, (end - Date.now()) / 1000);
            c.querySelector('b').textContent = Math.ceil(left);
            c.style.setProperty('--p', `${last ? (100 * left) / last.time : 0}`);
            c.classList.toggle('hurry', left <= 5);
        };
        setInterval(paintClock, 100);
        const paint = (d) => {
            const r = d.round && d.round.id === id ? d.round : null;
            const was = phase;
            phase = !r ? 'ready' : r.open ? 'open' : 'done';
            last = r;
            card.classList.toggle('go', phase !== 'ready');
            card.classList.toggle('done', phase === 'done');
            const lis = card.querySelectorAll('.quiz-opts li');
            lis.forEach((li, i) => {
                li.classList.toggle('right', phase === 'done' && i === r.answer);
                li.classList.toggle('off', phase === 'done' && i !== r.answer);
                const n = li.querySelector('.qn');
                const total = phase === 'done' ? r.counts.reduce((a, b) => a + b, 0) : 0;
                n.innerHTML = phase === 'done' ? `${r.counts[i]}<small>${total ? Math.round((100 * r.counts[i]) / total) : 0}%</small>` : '';
                li.style.setProperty('--share', phase === 'done' && total ? `${(100 * r.counts[i]) / total}%` : '0%');
            });
            if (phase === 'ready') {
                hud.innerHTML = `<span class="qh-go"><kbd>Enter</kbd> START THE QUIZ</span><span class="qh-n">${d.players || 0} player${d.players === 1 ? '' : 's'}</span>`;
            } else if (phase === 'open') {
                end = Date.now() + r.left * 1000;
                if (!hud.querySelector('.qh-clock')) {
                    hud.innerHTML = `<span class="qh-clock"><b></b></span><span class="qh-n"></span><span class="qh-hint"><kbd>Enter</kbd> close now</span>`;
                }
                hud.querySelector('.qh-n').innerHTML = `<b>${r.answered}</b> / ${r.players} answered`;
                paintClock();
            } else {
                const fast = (r.fastest || []).map((f, i) => `<li><span>${['🥇', '🥈', '🥉'][i]}</span>${escHtml(f.name)} <small>${(f.ms / 1000).toFixed(1)} s</small></li>`).join('');
                hud.innerHTML = `${fast ? `<ol class="qh-fast">${fast}</ol>` : '<span class="qh-n">Nobody got it right</span>'}
                    <span class="qh-hint"><kbd>Enter</kbd> leaderboard · <kbd>⇧ Enter</kbd> play again</span>`;
                if (was === 'open' && soundOn()) withAudio(moduleSting);
            }
            if (boardOv.classList.contains('open')) drawBoard(d);
        };
        const offline = () => {
            phase = 'offline';
            card.classList.remove('go', 'done');
            hud.innerHTML = '<span class="qh-n">The quiz needs the room: respawn serve --audience (or --tunnel)</span>';
        };
        const refresh = () => {
            clearTimeout(timer);
            pollFetch('/__quiz').then(paint).catch(offline)
                .finally(() => { timer = setTimeout(refresh, phase === 'open' ? 300 : 1500); });
        };
        refresh();
        const start = () => pollFetch('/__quiz/start', { id }).then((d) => {
            if (soundOn()) withAudio(partSting);
            paint(d);
            refresh();
        }).catch(offline);
        shortcuts.Enter = (ev) => {
            if (phase === 'ready' || phase === 'offline' || (phase === 'done' && ev.shiftKey)) start();
            else if (phase === 'open') pollFetch('/__quiz/stop', {}).then(paint).catch(() => {});
            else if (phase === 'done') openBoard();
        };
    }
}

// --- the room's phones
if (AUD && top) {
    const ov = el('div', 'quiz-phone', '');
    document.body.appendChild(ov);
    let d = null, end = 0, timer = 0, busy = false, msg = '';
    const onQuizSlide = () => Boolean(top.querySelector('.quiz'));
    const DISMISS = `${NS}.quiz.seen`;
    const paintClock = () => {
        const b = ov.querySelector('.qp-bar i');
        const n = ov.querySelector('.qp-left');
        const r = d && d.round;
        if (!b || !r) return;
        const left = Math.max(0, (end - Date.now()) / 1000);
        b.style.width = `${(100 * left) / r.time}%`;
        n.textContent = Math.ceil(left);
        ov.classList.toggle('hurry', left <= 5);
    };
    setInterval(paintClock, 100);
    const sigOf = (s) => (ov.dataset.sig === s ? false : (ov.dataset.sig = s, true));
    const me = () => (d && d.name ? `<div class="qp-me"><b>${escHtml(d.name)}</b><span>${(d.score || 0).toLocaleString()} pts${d.rank ? ` · #${d.rank} of ${d.players}` : ''}</span><span>${'🃏'.repeat(d.jokers || 0)}</span></div>` : '');
    const draw = () => {
        const r = d && d.round;
        const open = r && r.open;
        let view = 'none';
        if (d && d.board && d.leaders) view = 'board';
        else if (open && d.name === null) view = 'name';
        else if (open) view = r.mine === undefined ? 'answer' : 'locked';
        else if (r && !open && store.get(DISMISS, 0) !== r.n && (r.result || r.slide === slideNo)) view = 'result';
        else if (onQuizSlide() && d && d.name === null) view = 'name';
        else if (onQuizSlide() && d && d.name) view = 'wait';
        ov.classList.toggle('open', view !== 'none');
        ov.dataset.view = view;
        if (view === 'none') { ov.dataset.sig = ''; return; }
        if (view === 'name') {
            if (!sigOf(`name|${msg}`)) return;
            ov.innerHTML = `<form class="qp-card qp-name"><span class="qp-tag">QUIZ</span>
                <h2>Your player name</h2><input name="n" maxlength="24" autocomplete="nickname" value="${escHtml(store.get(`${NS}.quiz.name`, ''))}" placeholder="e.g. Ada" required>
                <button type="submit">I'm in</button><p class="qp-msg">${escHtml(msg)}</p></form>`;
            ov.querySelector('form').addEventListener('submit', (ev) => {
                ev.preventDefault();
                const name = ov.querySelector('input').value.trim();
                if (!name) return;
                store.set(`${NS}.quiz.name`, name);
                pollFetch('/__quiz/name', { name }).then((res) => { msg = res.ok ? '' : res.error || 'Try another name'; tick(); })
                    .catch(() => { msg = 'Not sent: try again'; draw(); });
            });
            return;
        }
        if (view === 'answer' || view === 'locked') {
            if (sigOf(`${view}|${r.n}|${r.mine}`)) {
                ov.innerHTML = `<div class="qp-card qp-play"><div class="qp-top"><span class="qp-tag">QUESTION ${r.n}</span><span class="qp-left"></span></div>
                    <div class="qp-bar"><i></i></div><h2 class="qp-q">${r.q || ''}</h2>
                    <div class="qp-opts">${(r.options || []).map((o, i) => `<button type="button" data-c="${i}" data-color="${QUIZ_COLORS[i % QUIZ_COLORS.length]}" class="${r.mine === i ? 'mine' : ''}" ${view === 'locked' ? 'disabled' : ''}><span class="ql">${LETTERS[i]}</span><span>${o}</span></button>`).join('')}</div>
                    <p class="qp-msg">${view === 'locked' ? `Locked in: <b>${LETTERS[r.mine]}</b>. Wait for the others…` : 'Fast and right scores more. A wrong answer costs points.'}</p>${me()}</div>`;
            }
            paintClock();
            return;
        }
        if (view === 'result') {
            const res = r.result;
            const kind = res ? res.kind : 'watch';
            if (!sigOf(`result|${r.n}|${kind}|${d.score}`)) return;
            const title = { right: 'RIGHT!', wrong: 'WRONG', joker: 'JOKER USED', missed: 'TOO LATE', watch: 'THE ANSWER' }[kind];
            const line = !res ? '' : kind === 'right' ? `${quizNum(res.delta)} <small>in ${(res.ms / 1000).toFixed(1)} s</small>`
                : kind === 'joker' ? `0 <small>${d.jokers} joker${d.jokers === 1 ? '' : 's'} left</small>` : quizNum(res.delta);
            ov.innerHTML = `<div class="qp-card qp-result" data-kind="${kind}"><span class="qp-tag">QUESTION ${r.n}</span>
                <h2>${title}</h2>${line ? `<p class="qp-delta">${line}</p>` : ''}
                <p class="qp-answer"><span class="ql">${LETTERS[r.answer]}</span>${(r.options || [])[r.answer] || ''}</p>
                ${r.explain ? `<p class="qp-explain">${r.explain}</p>` : ''}${me()}
                <button type="button" data-a="close">OK</button></div>`;
            return;
        }
        if (view === 'wait') {
            if (!sigOf(`wait|${d.name}|${d.score}|${d.rank}|${d.players}`)) return;
            ov.innerHTML = `<div class="qp-card qp-wait"><span class="qp-tag">QUIZ</span><h2>GET READY</h2>
                <p class="qp-msg">The options show up here when the trainer starts the question. Fast and right scores more; a wrong answer costs points; ${d.jokers || 0} joker${d.jokers === 1 ? '' : 's'} let you skip without losing any.</p>${me()}</div>`;
            return;
        }
        if (view === 'board') {
            if (!sigOf(`board|${JSON.stringify(d.leaders)}|${d.rank}`)) return;
            const mine = d.leaders.some((x) => x.me) || !d.name ? '' : `<p class="qp-msg">You: <b>#${d.rank}</b> · ${(d.score || 0).toLocaleString()} pts</p>`;
            ov.innerHTML = `<div class="qp-card qp-board"><span class="qp-tag">LEADERBOARD</span>${quizBoard(d.leaders, mine)}</div>`;
        }
    };
    ov.addEventListener('click', (ev) => {
        const b = ev.target.closest('button');
        if (!b || !d || !d.round) return;
        if (b.dataset.a === 'close') { store.set(DISMISS, d.round.n); draw(); return; }
        if (b.dataset.c === undefined || busy || d.round.mine !== undefined) return;
        busy = true;
        const choice = Number(b.dataset.c);
        b.classList.add('mine');
        pollFetch('/__quiz/answer', { n: d.round.n, choice })
            .then((res) => { if (res.ok) d.round.mine = choice; else { b.classList.remove('mine'); toast(escHtml(res.error || 'Not taken')); } draw(); })
            .catch(() => b.classList.remove('mine'))
            .finally(() => { busy = false; });
    });
    const tick = () => {
        clearTimeout(timer);
        pollFetch('/__quiz')
            .then((nd) => {
                if (!nd || nd.name === undefined) return;
                d = nd;
                if (d.round && d.round.open && typeof d.round.left === 'number') {
                    const e = Date.now() + d.round.left * 1000;
                    if (!end || Math.abs(e - end) > 400 || ov.dataset.view !== 'answer') end = e;
                }
                draw();
            })
            .catch(() => {})
            .finally(() => { timer = setTimeout(tick, (d && d.round && d.round.open) || onQuizSlide() ? 400 : 1500); });
    };
    tick();
}
