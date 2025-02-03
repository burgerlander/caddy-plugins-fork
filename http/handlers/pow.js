const fromHexString = hexString =>
  new Uint8Array(hexString.match(/.{1,2}/g).map(byte => parseInt(byte, 16)));

const toHexString = bytes =>
  bytes.reduce((str, byte) => str + byte.toString(16).padStart(2, '0'), '');

const seedStr = "{{ .Seed }}";
const seed = fromHexString(seedStr);
const target = "{{ .Target }}";

const fullBuf = new ArrayBuffer(seed.byteLength*2);

const fullBufSeed = new Uint8Array(fullBuf, 0, seed.byteLength);
seed.forEach((v, i) => fullBufSeed[i] = v);

const randBuf = new Uint8Array(fullBuf, seed.byteLength);

(async () => {
  while (true) {
    crypto.getRandomValues(randBuf);
    const digest = await crypto.subtle.digest('SHA-512', fullBuf);
    const digestView = new DataView(digest);
    if (digestView.getUint32(0) < target) {
      const solutionStr = toHexString(randBuf);
      document.cookie = `{{ .ChallengeSeedCookie }}=${seedStr}`;
      document.cookie = `{{ .ChallengeSolutionCookie }}=${solutionStr}`;
      window.location.reload();
    }
  }
})();
