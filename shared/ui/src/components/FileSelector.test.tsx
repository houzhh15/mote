import { describe, it, expect } from 'vitest';
import { detectFileType } from './FileSelector';

describe('FileSelector', () => {
  describe('detectFileType', () => {
    it('detects image files', () => {
      expect(detectFileType('photo.png')).toBe('image');
      expect(detectFileType('avatar.jpg')).toBe('image');
      expect(detectFileType('icon.svg')).toBe('image');
      expect(detectFileType('animation.gif')).toBe('image');
      expect(detectFileType('image.webp')).toBe('image');
    });

    it('detects code files', () => {
      expect(detectFileType('main.go')).toBe('code');
      expect(detectFileType('index.ts')).toBe('code');
      expect(detectFileType('App.tsx')).toBe('code');
      expect(detectFileType('script.py')).toBe('code');
      expect(detectFileType('Server.java')).toBe('code');
      expect(detectFileType('main.cpp')).toBe('code');
      expect(detectFileType('lib.rs')).toBe('code');
    });

    it('detects text files', () => {
      expect(detectFileType('README.md')).toBe('text');
      expect(detectFileType('data.json')).toBe('text');
      expect(detectFileType('config.yaml')).toBe('text');
      expect(detectFileType('config.yml')).toBe('text');
      expect(detectFileType('data.xml')).toBe('text');
      expect(detectFileType('notes.txt')).toBe('text');
      expect(detectFileType('Makefile')).toBe('text');
    });

    it('detects PDF files', () => {
      expect(detectFileType('document.pdf')).toBe('pdf');
      expect(detectFileType('REPORT.PDF')).toBe('pdf');
    });

    it('detects archive files', () => {
      expect(detectFileType('backup.zip')).toBe('archive');
      expect(detectFileType('release.tar.gz')).toBe('archive');
      expect(detectFileType('data.gz')).toBe('archive');
      expect(detectFileType('archive.7z')).toBe('archive');
      expect(detectFileType('package.rar')).toBe('archive');
    });

    it('returns "other" for unknown types', () => {
      expect(detectFileType('mystery.xyz')).toBe('other');
      expect(detectFileType('data.bin')).toBe('other');
      expect(detectFileType('file')).toBe('other');
    });

    it('handles case insensitivity', () => {
      expect(detectFileType('FILE.PNG')).toBe('image');
      expect(detectFileType('Code.GO')).toBe('code');
      expect(detectFileType('Doc.MD')).toBe('text');
    });

    it('handles files without extensions', () => {
      expect(detectFileType('Makefile')).toBe('text');
      expect(detectFileType('Dockerfile')).toBe('text');
      expect(detectFileType('LICENSE')).toBe('other');
    });

    it('handles multiple dots in filename', () => {
      expect(detectFileType('archive.tar.gz')).toBe('archive');
      expect(detectFileType('config.test.ts')).toBe('code');
    });
  });
});
