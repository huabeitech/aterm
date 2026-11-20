import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

func fail(_ message: String) -> Never {
    FileHandle.standardError.write(Data((message + "\n").utf8))
    exit(1)
}

guard CommandLine.arguments.count == 4 else {
    fail("Usage: swift compose.swift <window.png> <wallpaper> <output.png>")
}

func load(_ path: String) -> CGImage {
    guard let source = CGImageSourceCreateWithURL(URL(fileURLWithPath: path) as CFURL, nil),
          let image = CGImageSourceCreateImageAtIndex(source, CGImageSourceGetPrimaryImageIndex(source), nil) else {
        fail("Cannot read image: \(path)")
    }
    return image
}

let window = load(CommandLine.arguments[1])
let wallpaper = load(CommandLine.arguments[2])
let inset = 20
let width = window.width + inset * 2
let height = window.height + inset * 2
let context = CGContext(data: nil, width: width, height: height, bitsPerComponent: 8,
    bytesPerRow: 0, space: CGColorSpace(name: CGColorSpace.sRGB)!,
    bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
let scale = max(Double(width) / Double(wallpaper.width), Double(height) / Double(wallpaper.height))
let scaledWidth = Double(wallpaper.width) * scale
let scaledHeight = Double(wallpaper.height) * scale
context.interpolationQuality = .high
context.draw(wallpaper, in: CGRect(x: (Double(width) - scaledWidth) / 2,
    y: (Double(height) - scaledHeight) / 2, width: scaledWidth, height: scaledHeight))
context.interpolationQuality = .none
context.draw(window, in: CGRect(x: inset, y: inset, width: window.width, height: window.height))
let output = CGImageDestinationCreateWithURL(URL(fileURLWithPath: CommandLine.arguments[3]) as CFURL,
    UTType.png.identifier as CFString, 1, nil)!
CGImageDestinationAddImage(output, context.makeImage()!, nil)
precondition(CGImageDestinationFinalize(output))
print("\(window.width) × \(window.height) → \(width) × \(height), 20px padding")
