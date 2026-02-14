<script>
  import { onMount } from 'svelte';

  export let hueShift = 0;
  export let noiseIntensity = 0.05;
  export let scanlineIntensity = 0;
  export let speed = 0.5;
  export let scanlineFrequency = 0;
  export let warpAmount = 0;

  let canvas;
  let animationFrame;

  const vertex = `
    attribute vec2 position;
    void main() {
      gl_Position = vec4(position, 0.0, 1.0);
    }
  `;

  const fragment = `
    #ifdef GL_ES
    precision lowp float;
    #endif

    uniform vec2 uResolution;
    uniform float uTime;
    uniform float uHueShift;
    uniform float uNoise;
    uniform float uScan;
    uniform float uScanFreq;
    uniform float uWarp;

    float rand(vec2 c) {
      return fract(sin(dot(c, vec2(12.9898, 78.233))) * 43758.5453);
    }

    vec3 hsv2rgb(vec3 c) {
      vec4 K = vec4(1.0, 2.0 / 3.0, 1.0 / 3.0, 3.0);
      vec3 p = abs(fract(c.xxx + K.xyz) * 6.0 - K.www);
      return c.z * mix(K.xxx, clamp(p - K.xxx, 0.0, 1.0), c.y);
    }

    void main() {
      vec2 uv = gl_FragCoord.xy / uResolution.xy;
      uv = uv * 2.0 - 1.0;
      
      float t = uTime * 0.5;
      
      // Основной паттерн
      float d = length(uv) * 0.5;
      float pattern = sin(d * 10.0 + t) * 0.5 + 0.5;
      pattern += sin(uv.x * 5.0 + t) * 0.25;
      pattern += sin(uv.y * 5.0 + t * 0.7) * 0.25;
      
      // Деформация
      if (uWarp > 0.0) {
        uv += uWarp * vec2(sin(uv.y + t), cos(uv.x + t)) * 0.1;
      }
      
      // Цвет
      vec3 col = hsv2rgb(vec3(pattern + uHueShift / 360.0, 0.8, 0.3 + pattern * 0.4));
      
      // Шум
      if (uNoise > 0.0) {
        col += (rand(gl_FragCoord.xy + t) - 0.5) * uNoise;
      }
      
      // Сканлинии
      if (uScan > 0.0 && uScanFreq > 0.0) {
        float scanline = sin(gl_FragCoord.y * uScanFreq) * 0.5 + 0.5;
        col *= 1.0 - (scanline * scanline) * uScan;
      }
      
      gl_FragColor = vec4(clamp(col, 0.0, 1.0), 1.0);
    }
  `;

  function createShader(gl, source, type) {
    const shader = gl.createShader(type);
    gl.shaderSource(shader, source);
    gl.compileShader(shader);
    if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
      console.error(gl.getShaderInfoLog(shader));
    }
    return shader;
  }

  function initWebGL() {
    if (!canvas) return;
    const gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl');
    if (!gl) {
      console.error('WebGL not supported');
      return null;
    }

    const vs = createShader(gl, vertex, gl.VERTEX_SHADER);
    const fs = createShader(gl, fragment, gl.FRAGMENT_SHADER);

    const program = gl.createProgram();
    gl.attachShader(program, vs);
    gl.attachShader(program, fs);
    gl.linkProgram(program);

    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
      console.error(gl.getProgramInfoLog(program));
    }

    gl.useProgram(program);

    const positionBuffer = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, positionBuffer);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]), gl.STATIC_DRAW);

    const positionLoc = gl.getAttribLocation(program, 'position');
    gl.enableVertexAttribArray(positionLoc);
    gl.vertexAttribPointer(positionLoc, 2, gl.FLOAT, false, 0, 0);

    return { gl, program };
  }

  onMount(() => {
    const parent = canvas.parentElement;
    const ctx = initWebGL();
    if (!ctx) return;

    const { gl, program } = ctx;
    const resolutionLoc = gl.getUniformLocation(program, 'uResolution');
    const timeLoc = gl.getUniformLocation(program, 'uTime');
    const hueShiftLoc = gl.getUniformLocation(program, 'uHueShift');
    const noiseLoc = gl.getUniformLocation(program, 'uNoise');
    const scanLoc = gl.getUniformLocation(program, 'uScan');
    const scanFreqLoc = gl.getUniformLocation(program, 'uScanFreq');
    const warpLoc = gl.getUniformLocation(program, 'uWarp');

    const resize = () => {
      const w = parent.clientWidth;
      const h = parent.clientHeight;
      canvas.width = w;
      canvas.height = h;
      gl.viewport(0, 0, w, h);
      gl.uniform2f(resolutionLoc, w, h);
    };

    window.addEventListener('resize', resize);
    resize();

    const start = performance.now();

    const loop = () => {
      const time = ((performance.now() - start) / 1000) * speed;
      gl.uniform1f(timeLoc, time);
      gl.uniform1f(hueShiftLoc, hueShift);
      gl.uniform1f(noiseLoc, noiseIntensity);
      gl.uniform1f(scanLoc, scanlineIntensity);
      gl.uniform1f(scanFreqLoc, scanlineFrequency);
      gl.uniform1f(warpLoc, warpAmount);
      gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
      animationFrame = requestAnimationFrame(loop);
    };

    loop();

    return () => {
      cancelAnimationFrame(animationFrame);
      window.removeEventListener('resize', resize);
    };
  });
</script>

<style>
  canvas {
    display: block;
    width: 100%;
    height: 100%;
  }
</style>

<canvas bind:this={canvas} />
