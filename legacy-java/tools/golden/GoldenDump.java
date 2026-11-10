// GoldenDump writes the reference data the Go rewrite is tested against.
//
// It runs the Java Briss pipeline step by step on one PDF and prints JSON with
// the clusters, the sampled pages, the exact column and row sums of every
// cluster overlay, the legacy auto-crop and split ratios, and the page boxes
// of the cropped output for the plain and the column-split variants.
// "geom" mode prints RectangleHandler results for a table of boxes, ratios
// and rotations. Not part of the Gradle build; see run.sh.
//
//   java GoldenDump pdf <file.pdf> <password|-> <workdir>   > golden.json
//   java GoldenDump geom                                     > geom.json

import at.laborg.briss.model.ClusterDefinition;
import at.laborg.briss.model.CropDefinition;
import at.laborg.briss.model.CropFinder;
import at.laborg.briss.model.PageCluster;
import at.laborg.briss.model.SplitFinder;
import at.laborg.briss.utils.ClusterCreator;
import at.laborg.briss.utils.ClusterRenderWorker;
import at.laborg.briss.utils.DocumentCropper;
import at.laborg.briss.utils.PDFReaderUtil;
import at.laborg.briss.utils.RectangleHandler;
import com.itextpdf.text.Rectangle;
import com.itextpdf.text.pdf.PdfReader;
import java.awt.image.BufferedImage;
import java.awt.image.Raster;
import java.io.File;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import javax.imageio.ImageIO;

public final class GoldenDump {

	public static void main(String[] args) throws Exception {
		if (args.length == 1 && args[0].equals("geom")) {
			System.out.println(geom());
			return;
		}
		if (args.length != 4 || !args[0].equals("pdf")) {
			System.err.println("usage: GoldenDump pdf <file.pdf> <password|-> <workdir> | GoldenDump geom");
			System.exit(1);
		}
		File src = new File(args[1]);
		String pw = args[2].equals("-") ? null : args[2];
		File work = new File(args[3]);
		work.mkdirs();
		System.out.println(pdf(src, pw, work));
	}

	private static String pdf(File src, String pw, File work) throws Exception {
		ClusterDefinition cd = ClusterCreator.clusterPages(src, pw, null);
		new ClusterRenderWorker(src, pw, cd).run();
		PdfReader in = PDFReaderUtil.getPdfReader(src.getAbsolutePath(), pw);
		int pages = in.getNumberOfPages();
		in.close();

		StringBuilder sb = new StringBuilder();
		sb.append("{\n \"file\": ").append(q(src.getName())).append(",\n \"pages\": ").append(pages);
		sb.append(",\n \"clusters\": [");
		List<float[]> autos = new ArrayList<>();
		List<List<float[]>> splitCols = new ArrayList<>();
		int ci = 0;
		for (PageCluster c : cd.getClusterList()) {
			BufferedImage img = c.getImageData().getPreviewImage();
			ImageIO.write(img, "png", new File(work, src.getName() + "_c" + ci + ".png"));
			Raster r = img.getRaster();
			int w = img.getWidth(), h = img.getHeight();
			long[] cols = new long[w], rows = new long[h];
			for (int y = 0; y < h; y++) {
				for (int x = 0; x < w; x++) {
					int v = r.getSample(x, y, 0);
					cols[x] += v;
					rows[y] += v;
				}
			}
			float[] auto = CropFinder.getAutoCropFloats(img);
			List<float[]> sc = SplitFinder.splitColumn(img, auto);
			List<float[]> sr = SplitFinder.splitRow(img, auto);
			autos.add(auto);
			splitCols.add(sc);
			sb.append(ci == 0 ? "\n" : ",\n");
			sb.append("  {\"even\": ").append(c.isEvenPage());
			sb.append(", \"rounded_size\": [").append(c.getRoundedPageWidth()).append(", ").append(c.getRoundedPageHeight()).append("]");
			sb.append(",\n   \"pages\": ").append(ints(c.getAllPages()));
			sb.append(",\n   \"sample\": ").append(ints(c.getPagesToMerge()));
			sb.append(",\n   \"overlay\": {\"w\": ").append(w).append(", \"h\": ").append(h);
			sb.append(",\n    \"col_sums\": ").append(Arrays.toString(cols));
			sb.append(",\n    \"row_sums\": ").append(Arrays.toString(rows)).append("}");
			sb.append(",\n   \"autocrop\": ").append(ratio(auto));
			sb.append(",\n   \"split_col\": [").append(ratio(sc.get(0))).append(", ").append(ratio(sc.get(1))).append("]");
			sb.append(",\n   \"split_row\": [").append(ratio(sr.get(0))).append(", ").append(ratio(sr.get(1))).append("]}");
			ci++;
		}
		sb.append("\n ],\n \"output\": {");
		for (int variant = 0; variant < 2; variant++) {
			int k = 0;
			for (PageCluster c : cd.getClusterList()) {
				c.clearRatios();
				if (variant == 0) {
					c.addRatios(autos.get(k));
				} else {
					for (float[] f : splitCols.get(k)) {
						c.addRatios(f);
					}
				}
				k++;
			}
			File dest = new File(work, src.getName() + (variant == 0 ? "_plain.pdf" : "_split_col.pdf"));
			dest.delete();
			sb.append(variant == 0 ? "\n  \"plain\": " : ",\n  \"split_col\": ");
			try {
				CropDefinition def = CropDefinition.createCropDefinition(src, dest, cd);
				DocumentCropper.crop(def, pw);
				PdfReader out = PDFReaderUtil.getPdfReader(dest.getAbsolutePath(), pw);
				sb.append("[");
				for (int p = 1; p <= out.getNumberOfPages(); p++) {
					sb.append(p == 1 ? "\n   " : ",\n   ");
					sb.append("{\"media\": ").append(box(out.getBoxSize(p, "media")));
					sb.append(", \"crop\": ").append(box(out.getBoxSize(p, "crop")));
					sb.append(", \"rotation\": ").append(out.getPageRotation(p)).append("}");
				}
				sb.append("]");
				out.close();
			} catch (Exception e) {
				sb.append("{\"error\": ").append(q(e.toString())).append("}");
			}
		}
		sb.append("\n }\n}");
		return sb.toString();
	}

	private static String geom() {
		float[][] media = {{0, 0, 612, 792}, {0, 0, 612, 792}, {-10, -20, 600, 800}, {0, 0, 960, 540}};
		float[][] crop = {null, {36, 36, 576, 756}, {0, 0, 590, 790}, {0, 0, 1000, 1000}};
		float[][] ratios = {{0, 0, 0, 0}, {0.1f, 0.2f, 0.05f, 0.15f}, {0.25f, 0.1f, 0.3f, 0.4f}, {0.0123f, 0.5f, 0.4567f, 0.01f}};
		int[] rotations = {0, 90, 180, 270};
		StringBuilder sb = new StringBuilder("[");
		boolean first = true;
		for (int b = 0; b < media.length; b++) {
			for (float[] r : ratios) {
				for (int rot : rotations) {
					List<Rectangle> boxes = new ArrayList<>();
					boxes.add(new Rectangle(media[b][0], media[b][1], media[b][2], media[b][3]));
					if (crop[b] != null) {
						boxes.add(new Rectangle(crop[b][0], crop[b][1], crop[b][2], crop[b][3]));
					}
					Rectangle res = RectangleHandler.calculateScaledRectangle(boxes, r, rot);
					sb.append(first ? "\n " : ",\n ");
					first = false;
					sb.append("{\"media\": ").append(arr(media[b])).append(", \"crop\": ").append(crop[b] == null ? "null" : arr(crop[b]));
					sb.append(", \"ratios\": ").append(ratio(r)).append(", \"rotation\": ").append(rot);
					sb.append(", \"result\": ").append(box(res)).append("}");
				}
			}
		}
		return sb.append("\n]").toString();
	}

	private static String ratio(float[] f) {
		return "{\"l\": " + f[0] + ", \"b\": " + f[1] + ", \"r\": " + f[2] + ", \"t\": " + f[3] + "}";
	}

	private static String box(Rectangle r) {
		return "[" + r.getLeft() + ", " + r.getBottom() + ", " + r.getRight() + ", " + r.getTop() + "]";
	}

	private static String arr(float[] f) {
		return "[" + f[0] + ", " + f[1] + ", " + f[2] + ", " + f[3] + "]";
	}

	private static String ints(List<Integer> l) {
		return l.toString();
	}

	private static String q(String s) {
		return "\"" + s.replace("\\", "\\\\").replace("\"", "\\\"") + "\"";
	}
}
